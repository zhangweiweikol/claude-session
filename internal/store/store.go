// Author: zhangweiwei
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"claude-session/internal/model"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS projects (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  cwd          TEXT NOT NULL UNIQUE,
  encoded_dir  TEXT NOT NULL,
  name         TEXT NOT NULL,
  last_scan_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS folders (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  parent_id  INTEGER REFERENCES folders(id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_folders_project ON folders(project_id);

CREATE TABLE IF NOT EXISTS sessions (
  id            TEXT PRIMARY KEY,
  project_id    INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  folder_id     INTEGER REFERENCES folders(id) ON DELETE SET NULL,
  title         TEXT NOT NULL,
  ai_title      TEXT NOT NULL DEFAULT '',
  custom_title  TEXT NOT NULL DEFAULT '',
  last_prompt   TEXT NOT NULL DEFAULT '',
  message_count INTEGER NOT NULL DEFAULT 0,
  file_path     TEXT NOT NULL,
  file_size     INTEGER NOT NULL DEFAULT 0,
  file_mtime    INTEGER NOT NULL DEFAULT 0,
  first_ts      TEXT NOT NULL DEFAULT '',
  last_ts       TEXT NOT NULL DEFAULT '',
  git_branch    TEXT NOT NULL DEFAULT '',
  cc_version    TEXT NOT NULL DEFAULT '',
  state         TEXT NOT NULL DEFAULT 'ok',
  parent_id     TEXT NOT NULL DEFAULT '',
  fork_point_uuid TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_id, last_ts DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_folder ON sessions(folder_id);
`)
	if err != nil {
		return err
	}

	if !s.hasColumn("sessions", "deleted_at") {
		if _, err := s.db.Exec(`ALTER TABLE sessions ADD COLUMN deleted_at INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if !s.hasColumn("sessions", "parent_id") {
		if _, err := s.db.Exec(`ALTER TABLE sessions ADD COLUMN parent_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if !s.hasColumn("sessions", "fork_point_uuid") {
		if _, err := s.db.Exec(`ALTER TABLE sessions ADD COLUMN fork_point_uuid TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
		// 旧索引没有派生关系，置空 mtime 让下次扫描重新解析 jsonl。
		if _, err := s.db.Exec(`UPDATE sessions SET file_mtime = -1`); err != nil {
			return err
		}
	}
	if !s.hasColumn("projects", "pinned") {
		if _, err := s.db.Exec(`ALTER TABLE projects ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if !s.hasColumn("projects", "last_opened_at") {
		if _, err := s.db.Exec(`ALTER TABLE projects ADD COLUMN last_opened_at INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if !s.hasColumn("projects", "alias") {
		if _, err := s.db.Exec(`ALTER TABLE projects ADD COLUMN alias TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if !s.hasColumn("projects", "sort_order") {
		if _, err := s.db.Exec(`ALTER TABLE projects ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if !s.hasColumn("sessions", "tokens_input") {
		if _, err := s.db.Exec(`ALTER TABLE sessions ADD COLUMN tokens_input INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
		if _, err := s.db.Exec(`UPDATE sessions SET file_mtime = -1`); err != nil {
			return err
		}
	}
	if !s.hasColumn("sessions", "tokens_output") {
		if _, err := s.db.Exec(`ALTER TABLE sessions ADD COLUMN tokens_output INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
		if _, err := s.db.Exec(`UPDATE sessions SET file_mtime = -1`); err != nil {
			return err
		}
	}
	if !s.hasColumn("sessions", "display_project_id") {
		if _, err := s.db.Exec(`ALTER TABLE sessions ADD COLUMN display_project_id INTEGER REFERENCES projects(id) ON DELETE SET NULL`); err != nil {
			return err
		}
	}
	if !s.hasColumn("sessions", "display_project_source") {
		if _, err := s.db.Exec(`ALTER TABLE sessions ADD COLUMN display_project_source TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
		// 首次启用自动归属时强制重新解析一次，让已有会话按最近 cwd 归位。
		if _, err := s.db.Exec(`UPDATE sessions SET file_mtime = -1`); err != nil {
			return err
		}
	}

	// trigram 对中文子串搜索友好；若当前 SQLite 构建不支持则退回 unicode61。
	if err := s.createFTS(); err != nil {
		return err
	}
	// 旧库的全文表没有 summary 列，需要重建并强制重新索引。
	if !s.hasColumn("sessions_fts", "summary") {
		if _, err := s.db.Exec(`DROP TABLE sessions_fts`); err != nil {
			return err
		}
		if err := s.createFTS(); err != nil {
			return err
		}
		if _, err := s.db.Exec(`UPDATE sessions SET file_mtime = -1`); err != nil {
			return err
		}
	}
	// 旧库通过 ALTER 补列后才允许建索引，新库在建表语句里已包含该列。
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_parent ON sessions(parent_id)`); err != nil {
		return err
	}

	// AI 概要、归类建议与任务状态。独立于 sessions，避免扫描 upsert 覆盖派生数据。
	if _, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS session_ai (
  session_id      TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
  summary         TEXT NOT NULL DEFAULT '',
  suggested_title TEXT NOT NULL DEFAULT '',
  topics          TEXT NOT NULL DEFAULT '',
  model           TEXT NOT NULL DEFAULT '',
  profile         TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL DEFAULT 'pending',
  error           TEXT NOT NULL DEFAULT '',
  confidence      REAL NOT NULL DEFAULT 0,
  lineage_mode    TEXT NOT NULL DEFAULT '',
  drift           REAL NOT NULL DEFAULT 0,
  source_mtime    INTEGER NOT NULL DEFAULT 0,
  source_size     INTEGER NOT NULL DEFAULT 0,
  in_tokens       INTEGER NOT NULL DEFAULT 0,
  out_tokens      INTEGER NOT NULL DEFAULT 0,
  cost_usd        REAL NOT NULL DEFAULT 0,
  attempts        INTEGER NOT NULL DEFAULT 0,
  updated_at      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_session_ai_status ON session_ai(status);

CREATE TABLE IF NOT EXISTS ai_jobs (
  id         TEXT PRIMARY KEY,
  kind       TEXT NOT NULL,
  project_id INTEGER NOT NULL DEFAULT 0,
  status     TEXT NOT NULL,
  total      INTEGER NOT NULL DEFAULT 0,
  done       INTEGER NOT NULL DEFAULT 0,
  failed     INTEGER NOT NULL DEFAULT 0,
  error      TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ai_jobs_project ON ai_jobs(project_id, kind, status);

CREATE TABLE IF NOT EXISTS ai_suggestions (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id      TEXT NOT NULL,
  session_id  TEXT NOT NULL,
  folder_id   INTEGER NOT NULL DEFAULT 0,
  new_folder  TEXT NOT NULL DEFAULT '',
  folder_path TEXT NOT NULL DEFAULT '',
  action      TEXT NOT NULL,
  confidence  REAL NOT NULL DEFAULT 0,
  reason      TEXT NOT NULL DEFAULT '',
  applied     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_ai_suggestions_job ON ai_suggestions(job_id);

CREATE TABLE IF NOT EXISTS ai_folder_suggestions (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id      TEXT NOT NULL,
  parent_id   INTEGER NOT NULL DEFAULT 0,
  parent_name TEXT NOT NULL DEFAULT '',
  parent_path TEXT NOT NULL DEFAULT '',
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  confidence  REAL NOT NULL DEFAULT 0,
  applied     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_ai_folder_suggestions_job ON ai_folder_suggestions(job_id);

CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`); err != nil {
		return err
	}

	// 归类来源：manual 的会话永不参与自动移动，ai 的会话可被后续整理更新。
	if !s.hasColumn("sessions", "folder_source") {
		if _, err := s.db.Exec(`ALTER TABLE sessions ADD COLUMN folder_source TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if !s.hasColumn("folders", "source") {
		if _, err := s.db.Exec(`ALTER TABLE folders ADD COLUMN source TEXT NOT NULL DEFAULT 'user'`); err != nil {
			return err
		}
	}
	if !s.hasColumn("folders", "description") {
		if _, err := s.db.Exec(`ALTER TABLE folders ADD COLUMN description TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	// 归类建议的结构化引用：已有归类用 folder_id，本批新建用 new_folder / parent_name。
	if !s.hasColumn("ai_suggestions", "folder_id") {
		if _, err := s.db.Exec(`ALTER TABLE ai_suggestions ADD COLUMN folder_id INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if !s.hasColumn("ai_suggestions", "new_folder") {
		if _, err := s.db.Exec(`ALTER TABLE ai_suggestions ADD COLUMN new_folder TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if !s.hasColumn("ai_folder_suggestions", "parent_id") {
		if _, err := s.db.Exec(`ALTER TABLE ai_folder_suggestions ADD COLUMN parent_id INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if !s.hasColumn("ai_folder_suggestions", "parent_name") {
		if _, err := s.db.Exec(`ALTER TABLE ai_folder_suggestions ADD COLUMN parent_name TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	return nil
}

// createFTS 建立包含概要列的全文索引，trigram 不可用时退回 unicode61。
func (s *Store) createFTS() error {
	if _, err := s.db.Exec(`
CREATE VIRTUAL TABLE IF NOT EXISTS sessions_fts USING fts5(
  session_id UNINDEXED,
  title,
  last_prompt,
  summary,
  content,
  tokenize='trigram'
);`); err != nil {
		if _, err := s.db.Exec(`
CREATE VIRTUAL TABLE IF NOT EXISTS sessions_fts USING fts5(
  session_id UNINDEXED,
  title,
  last_prompt,
  summary,
  content,
  tokenize='unicode61'
);`); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) hasColumn(table, column string) bool {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var dflt any
		if rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk) == nil && name == column {
			return true
		}
	}
	return false
}

func (s *Store) UpsertProject(ctx context.Context, cwd, encodedDir, name string, now int64) (int64, error) {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO projects(cwd, encoded_dir, name, last_scan_at) VALUES(?,?,?,?)
ON CONFLICT(cwd) DO UPDATE SET
  encoded_dir = excluded.encoded_dir,
  name = excluded.name,
  last_scan_at = excluded.last_scan_at`,
		cwd, encodedDir, name, now)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.db.QueryRowContext(ctx, `SELECT id FROM projects WHERE cwd = ?`, cwd).Scan(&id)
	return id, err
}

func (s *Store) UpsertSession(ctx context.Context, sess model.Session) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO sessions(
  id, project_id, title, ai_title, custom_title, last_prompt, message_count,
  tokens_input, tokens_output,
  file_path, file_size, file_mtime, first_ts, last_ts, git_branch, cc_version, state,
  parent_id, fork_point_uuid
) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'ok',?,?)
ON CONFLICT(id) DO UPDATE SET
  project_id = excluded.project_id,
  title = excluded.title,
  ai_title = excluded.ai_title,
  custom_title = excluded.custom_title,
  last_prompt = excluded.last_prompt,
  message_count = excluded.message_count,
  tokens_input = excluded.tokens_input,
  tokens_output = excluded.tokens_output,
  file_path = excluded.file_path,
  file_size = excluded.file_size,
  file_mtime = excluded.file_mtime,
  first_ts = excluded.first_ts,
  last_ts = excluded.last_ts,
  git_branch = excluded.git_branch,
  cc_version = excluded.cc_version,
  state = 'ok',
  deleted_at = 0,
  parent_id = excluded.parent_id,
  fork_point_uuid = excluded.fork_point_uuid`,
		sess.ID, sess.ProjectID, sess.Title, sess.AITitle, sess.CustomTitle, sess.LastPrompt, sess.MessageCount,
		sess.TokensInput, sess.TokensOutput,
		sess.FilePath, sess.FileSize, sess.FileMtime, sess.FirstTS, sess.LastTS, sess.GitBranch, sess.CCVersion,
		sess.ParentID, sess.ForkPointUUID)
	return err
}

func (s *Store) FileState(ctx context.Context, id string) (size, mtime int64, state string, ok bool, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT file_size, file_mtime, state FROM sessions WHERE id = ?`, id).Scan(&size, &mtime, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, "", false, nil
	}
	if err != nil {
		return 0, 0, "", false, err
	}
	return size, mtime, state, true, nil
}

func (s *Store) MarkMissing(ctx context.Context, seen map[string]bool) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id, state FROM sessions WHERE state = 'ok'`)
	if err != nil {
		return err
	}
	type row struct {
		id    string
		state string
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.state); err != nil {
			rows.Close()
			return err
		}
		all = append(all, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range all {
		if seen[r.id] {
			continue
		}
		if r.state == "missing" {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE sessions SET state = 'missing' WHERE id = ?`, r.id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListProjects(ctx context.Context) ([]model.Project, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT p.id, p.cwd, p.encoded_dir, COALESCE(NULLIF(p.alias, ''), p.name), p.alias, p.name,
       p.last_scan_at, COUNT(s.id), p.pinned, p.last_opened_at
FROM projects p
LEFT JOIN sessions s ON COALESCE(s.display_project_id, s.project_id) = p.id AND s.state = 'ok'
GROUP BY p.id
ORDER BY p.pinned DESC, p.sort_order ASC, COUNT(s.id) DESC, p.cwd ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.Project, 0)
	for rows.Next() {
		var p model.Project
		if err := rows.Scan(&p.ID, &p.Cwd, &p.EncodedDir, &p.Name, &p.Alias, &p.OriginalName,
			&p.LastScanAt, &p.SessionCount, &p.Pinned, &p.LastOpenedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) ListFolders(ctx context.Context, projectID int64) ([]model.Folder, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, project_id, parent_id, name, sort_order, source, description
FROM folders WHERE project_id = ?
ORDER BY sort_order ASC, name ASC, id ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Folder
	for rows.Next() {
		var f model.Folder
		var parent sql.NullInt64
		if err := rows.Scan(&f.ID, &f.ProjectID, &parent, &f.Name, &f.SortOrder, &f.Source, &f.Description); err != nil {
			return nil, err
		}
		if parent.Valid {
			v := parent.Int64
			f.ParentID = &v
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) CreateFolder(ctx context.Context, projectID int64, name string, parentID *int64, now int64) (model.Folder, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.Folder{}, errors.New("文件夹名称不能为空")
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO folders(project_id, parent_id, name, sort_order, created_at)
VALUES(?,?,?,?,?)`, projectID, parentID, name, now, now)
	if err != nil {
		return model.Folder{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return model.Folder{}, err
	}
	return model.Folder{ID: id, ProjectID: projectID, ParentID: parentID, Name: name}, nil
}

const sessionColumns = `s.id, s.project_id, s.display_project_id, s.display_project_source, s.folder_id, s.title, s.ai_title, s.custom_title, s.last_prompt,
       s.message_count, s.tokens_input, s.tokens_output,
       s.file_path, s.file_size, s.file_mtime, s.first_ts, s.last_ts,
       s.git_branch, s.cc_version, s.state, s.deleted_at, p.cwd,
       s.parent_id, s.fork_point_uuid`

// sessionColumnsWithParent 在基础列之后追加父会话摘要，查询需 LEFT JOIN sessions ps。
const sessionColumnsWithParent = sessionColumns + `, COALESCE(ps.title, ''), COALESCE(ps.state, '')`

func (s *Store) ListSessions(ctx context.Context, f model.SessionFilter) ([]model.Session, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 200
	}
	state := "ok"
	if f.Deleted {
		state = "deleted"
	}

	var where []string
	var args []any
	if f.FolderID != nil {
		// 归类 id 全局唯一，直接按归类过滤；会话的展示项目可能随 cwd 漂移，
		// 不应因此从归类里消失。
		where = append(where, "s.state = ?")
		args = append(args, state)
	} else {
		where = append(where, "COALESCE(s.display_project_id, s.project_id) = ?", "s.state = ?")
		args = append(args, f.ProjectID, state)
	}
	if f.Unfiled {
		where = append(where, "s.folder_id IS NULL")
	} else if f.Categorized {
		where = append(where, "s.folder_id IS NOT NULL")
	} else if f.FolderID != nil {
		where = append(where, "s.folder_id = ?")
		args = append(args, *f.FolderID)
	}

	// trigram 检索要求至少 3 个字符，短查询继续走 LIKE。
	if q := strings.TrimSpace(f.Query); q != "" {
		if utf8.RuneCountInString(q) >= 3 {
			if sessions, err := s.searchSessions(ctx, f, where, args); err == nil {
				s.attachAISummaries(ctx, sessions)
				return sessions, nil
			}
		}
		where = append(where, "(s.title LIKE ? OR s.last_prompt LIKE ? OR s.id LIKE ?)")
		like := "%" + q + "%"
		args = append(args, like, like, like)
	}
	args = append(args, f.Limit)

	query := `SELECT ` + sessionColumnsWithParent + `
FROM sessions s
LEFT JOIN sessions ps ON ps.id = s.parent_id
JOIN projects p ON p.id = s.project_id
WHERE ` + strings.Join(where, " AND ") + `
ORDER BY s.last_ts DESC, s.id ASC
LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.Session, 0)
	for rows.Next() {
		sess, err := scanSessionWithParent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.attachAISummaries(ctx, out)
	return out, nil
}

func (s *Store) searchSessions(ctx context.Context, f model.SessionFilter, where []string, args []any) ([]model.Session, error) {
	clauses := append([]string{"sessions_fts MATCH ?"}, where...)
	queryArgs := append([]any{ftsQuery(f.Query)}, args...)
	queryArgs = append(queryArgs, f.Limit)

	query := `SELECT ` + sessionColumnsWithParent + `,
       snippet(sessions_fts, -1, char(1), char(2), '…', 16)
FROM sessions s
JOIN sessions_fts ON sessions_fts.session_id = s.id
LEFT JOIN sessions ps ON ps.id = s.parent_id
JOIN projects p ON p.id = s.project_id
WHERE ` + strings.Join(clauses, " AND ") + `
ORDER BY s.last_ts DESC, s.id ASC
LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.Session, 0)
	for rows.Next() {
		sess, err := scanSessionWithSnippet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

func ftsQuery(q string) string {
	return `"` + strings.ReplaceAll(q, `"`, `""`) + `"`
}

func (s *Store) GetSession(ctx context.Context, id string) (model.Session, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT `+sessionColumnsWithParent+`
FROM sessions s
LEFT JOIN sessions ps ON ps.id = s.parent_id
JOIN projects p ON p.id = s.project_id
WHERE s.id = ?`, id)
	sess, err := scanSessionWithParent(row)
	if err != nil {
		return model.Session{}, err
	}
	list := []model.Session{sess}
	s.attachAISummaries(ctx, list)
	return list[0], nil
}

// Lineage 返回从根到父会话的链，用于详情页血缘导航。遇到缺失父或循环时停止。
func (s *Store) Lineage(ctx context.Context, id string) ([]model.Session, error) {
	var out []model.Session
	seen := map[string]bool{id: true}
	cur := id
	for i := 0; i < 32; i++ {
		sess, err := s.GetSession(ctx, cur)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			return nil, err
		}
		if sess.ParentID == "" || seen[sess.ParentID] {
			break
		}
		seen[sess.ParentID] = true
		parent, err := s.GetSession(ctx, sess.ParentID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			return nil, err
		}
		out = append([]model.Session{parent}, out...)
		cur = parent.ID
	}
	return out, nil
}

func (s *Store) GetProject(ctx context.Context, id int64) (model.Project, error) {
	var p model.Project
	err := s.db.QueryRowContext(ctx, `
SELECT p.id, p.cwd, p.encoded_dir, COALESCE(NULLIF(p.alias, ''), p.name), p.alias, p.name, p.last_scan_at,
       (SELECT COUNT(1) FROM sessions s WHERE COALESCE(s.display_project_id, s.project_id) = p.id AND s.state = 'ok'),
       p.pinned, p.last_opened_at
FROM projects p WHERE p.id = ?`, id).Scan(
		&p.ID, &p.Cwd, &p.EncodedDir, &p.Name, &p.Alias, &p.OriginalName,
		&p.LastScanAt, &p.SessionCount, &p.Pinned, &p.LastOpenedAt)
	return p, err
}

func (s *Store) SetProjectAlias(ctx context.Context, id int64, alias string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE projects SET alias = ? WHERE id = ?`, alias, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) TouchProject(ctx context.Context, id, now int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE projects SET last_opened_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SetProjectPinned(ctx context.Context, id int64, pinned bool) error {
	value := 0
	if pinned {
		value = 1
	}
	// 新收藏的项目排到收藏组末尾，已有排序保持不变。
	res, err := s.db.ExecContext(ctx, `
UPDATE projects SET pinned = ?,
  sort_order = CASE WHEN pinned = 1 THEN sort_order
    ELSE (SELECT COALESCE(MAX(sort_order), -1) + 1 FROM projects WHERE pinned = 1) END
WHERE id = ?`, value, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// MovePinnedProject 在收藏项目之间上下移动一位，direction 取 up / down。
func (s *Store) MovePinnedProject(ctx context.Context, id int64, direction string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	ids, err := reorderIDs(ctx, tx, `
SELECT p.id
FROM projects p
LEFT JOIN sessions s ON COALESCE(s.display_project_id, s.project_id) = p.id AND s.state = 'ok'
WHERE p.pinned = 1
GROUP BY p.id
ORDER BY p.sort_order ASC, COUNT(s.id) DESC, p.cwd ASC`, nil, id, direction)
	if err != nil {
		return err
	}
	for i, pid := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE projects SET sort_order = ? WHERE id = ?`, i, pid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// MoveFolder 在同一个父归类的兄弟节点之间上下移动一位。
func (s *Store) MoveFolder(ctx context.Context, projectID, folderID int64, direction string) error {
	var parent sql.NullInt64
	if err := s.db.QueryRowContext(ctx,
		`SELECT parent_id FROM folders WHERE project_id = ? AND id = ?`, projectID, folderID).Scan(&parent); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `SELECT id FROM folders WHERE project_id = ? AND parent_id IS NULL ORDER BY sort_order ASC, name ASC, id ASC`
	args := []any{projectID}
	if parent.Valid {
		query = `SELECT id FROM folders WHERE project_id = ? AND parent_id = ? ORDER BY sort_order ASC, name ASC, id ASC`
		args = append(args, parent.Int64)
	}
	ids, err := reorderIDs(ctx, tx, query, args, folderID, direction)
	if err != nil {
		return err
	}
	for i, fid := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE folders SET sort_order = ? WHERE id = ?`, i, fid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// reorderIDs 按查询顺序读出兄弟节点 ID，把 target 与相邻节点交换后返回新顺序。
// 走到边界时原样返回，调用方按 0..n-1 重新编号即可。
func reorderIDs(ctx context.Context, tx *sql.Tx, query string, queryArgs []any, target int64, direction string) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	idx := -1
	for i, id := range ids {
		if id == target {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, sql.ErrNoRows
	}
	swap := idx - 1
	if direction == "down" {
		swap = idx + 1
	}
	if swap >= 0 && swap < len(ids) {
		ids[idx], ids[swap] = ids[swap], ids[idx]
	}
	return ids, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSession(row scanner) (model.Session, error) {
	var out model.Session
	var folder, display sql.NullInt64
	err := row.Scan(
		&out.ID, &out.ProjectID, &display, &out.DisplayProjectSource, &folder, &out.Title, &out.AITitle, &out.CustomTitle, &out.LastPrompt,
		&out.MessageCount, &out.TokensInput, &out.TokensOutput,
		&out.FilePath, &out.FileSize, &out.FileMtime, &out.FirstTS, &out.LastTS,
		&out.GitBranch, &out.CCVersion, &out.State, &out.DeletedAt, &out.Cwd,
		&out.ParentID, &out.ForkPointUUID,
	)
	if err != nil {
		return model.Session{}, err
	}
	if folder.Valid {
		v := folder.Int64
		out.FolderID = &v
	}
	if display.Valid {
		v := display.Int64
		out.DisplayProjectID = &v
	}
	return out, nil
}

// scanSessionWithParent 读取 sessionColumnsWithParent 的结果，附带父会话标题与状态。
func scanSessionWithParent(row scanner) (model.Session, error) {
	var out model.Session
	var folder, display sql.NullInt64
	err := row.Scan(
		&out.ID, &out.ProjectID, &display, &out.DisplayProjectSource, &folder, &out.Title, &out.AITitle, &out.CustomTitle, &out.LastPrompt,
		&out.MessageCount, &out.TokensInput, &out.TokensOutput,
		&out.FilePath, &out.FileSize, &out.FileMtime, &out.FirstTS, &out.LastTS,
		&out.GitBranch, &out.CCVersion, &out.State, &out.DeletedAt, &out.Cwd,
		&out.ParentID, &out.ForkPointUUID,
		&out.ParentTitle, &out.ParentState,
	)
	if err != nil {
		return model.Session{}, err
	}
	if folder.Valid {
		v := folder.Int64
		out.FolderID = &v
	}
	if display.Valid {
		v := display.Int64
		out.DisplayProjectID = &v
	}
	return out, nil
}

func scanSessionWithSnippet(row scanner) (model.Session, error) {
	var out model.Session
	var folder, display sql.NullInt64
	var snippet sql.NullString
	err := row.Scan(
		&out.ID, &out.ProjectID, &display, &out.DisplayProjectSource, &folder, &out.Title, &out.AITitle, &out.CustomTitle, &out.LastPrompt,
		&out.MessageCount, &out.TokensInput, &out.TokensOutput,
		&out.FilePath, &out.FileSize, &out.FileMtime, &out.FirstTS, &out.LastTS,
		&out.GitBranch, &out.CCVersion, &out.State, &out.DeletedAt, &out.Cwd,
		&out.ParentID, &out.ForkPointUUID,
		&out.ParentTitle, &out.ParentState, &snippet,
	)
	if err != nil {
		return model.Session{}, err
	}
	if folder.Valid {
		v := folder.Int64
		out.FolderID = &v
	}
	if display.Valid {
		v := display.Int64
		out.DisplayProjectID = &v
	}
	if snippet.Valid {
		out.Snippet = snippet.String
	}
	return out, nil
}

func (s *Store) SetSessionProject(ctx context.Context, id string, displayProjectID *int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET display_project_id = ?, display_project_source = 'manual', folder_id = NULL WHERE id = ?`,
		displayProjectID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SetSessionFollowAuto(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET display_project_id = NULL, display_project_source = '', folder_id = NULL WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ApplyAutoDisplayProject 只调整会话的展示项目，不清理归类。
// 继续对话时 cwd 会在项目子目录之间漂移，自动归属随之变化；归类属于会话所在项目，
// 不应因此丢失。真正需要清理归类的场景由用户手动切换归属时处理。
func (s *Store) ApplyAutoDisplayProject(ctx context.Context, id string, displayProjectID *int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET display_project_id = ? WHERE id = ? AND display_project_source <> 'manual'`,
		displayProjectID, id)
	return err
}

func (s *Store) InvalidateSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET file_mtime = -1 WHERE id = ?`, id)
	return err
}

func (s *Store) ClearDisplayProject(ctx context.Context, projectID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET display_project_id = NULL, display_project_source = 'manual' WHERE display_project_id = ?`, projectID)
	return err
}

func (s *Store) AutoDisplayAllowed(ctx context.Context, id string) (bool, error) {
	var source string
	err := s.db.QueryRowContext(ctx,
		`SELECT display_project_source FROM sessions WHERE id = ?`, id).Scan(&source)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return source != "manual", nil
}

func (s *Store) SetSessionFolder(ctx context.Context, id string, folderID *int64) error {
	// 用户手动移动：标记为 manual，AI 不再自动调整该会话的归类。
	res, err := s.db.ExecContext(ctx, `UPDATE sessions SET folder_id = ?, folder_source = 'manual' WHERE id = ?`, folderID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SessionFolder 返回会话的归类。会话不存在时 found=false，未归类时 folderID=nil。
func (s *Store) SessionFolder(ctx context.Context, id string) (folderID *int64, found bool, err error) {
	var folder sql.NullInt64
	err = s.db.QueryRowContext(ctx, `SELECT folder_id FROM sessions WHERE id = ?`, id).Scan(&folder)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if folder.Valid {
		v := folder.Int64
		return &v, true, nil
	}
	return nil, true, nil
}

// InheritFolderIfEmpty 仅在会话尚未归类时写入归类，避免覆盖用户手动调整。
func (s *Store) InheritFolderIfEmpty(ctx context.Context, id string, folderID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET folder_id = ?, folder_source = 'inherited' WHERE id = ? AND folder_id IS NULL`, folderID, id)
	return err
}

func (s *Store) SetSessionTitle(ctx context.Context, id, title string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE sessions SET title = ?, custom_title = ? WHERE id = ?`, title, title, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) RenameFolder(ctx context.Context, projectID, folderID int64, name string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE folders SET name = ? WHERE id = ? AND project_id = ?`, name, folderID, projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListProjectSessions(ctx context.Context, projectID int64) ([]model.Session, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT `+sessionColumns+`
FROM sessions s JOIN projects p ON p.id = s.project_id
WHERE s.project_id = ?`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.Session, 0)
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

func (s *Store) DeleteProject(ctx context.Context, projectID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM sessions_fts WHERE session_id IN (SELECT id FROM sessions WHERE project_id = ?)`,
		projectID); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func (s *Store) DeleteFolder(ctx context.Context, projectID, folderID int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM folders WHERE id = ? AND project_id = ?`, folderID, projectID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) FolderExists(ctx context.Context, projectID, folderID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM folders WHERE id = ? AND project_id = ?`, folderID, projectID).Scan(&n)
	return n > 0, err
}

func (s *Store) IndexSession(ctx context.Context, id, title, lastPrompt, content string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions_fts WHERE session_id = ?`, id); err != nil {
		return err
	}
	// 概要由 AI 异步生成，扫描阶段先索引正文；生成后通过 UpdateFTSSummary 回填。
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO sessions_fts(session_id, title, last_prompt, summary, content) VALUES(?,?,?,'',?)`,
		id, title, lastPrompt, content); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateFTSSummary 在概要生成后回填全文索引，让搜索能命中摘要。
func (s *Store) UpdateFTSSummary(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE sessions_fts
SET summary = COALESCE((SELECT ai.summary FROM session_ai ai WHERE ai.session_id = ? AND ai.status = 'ok'), '')
WHERE session_id = ?`, sessionID, sessionID)
	return err
}

// BackfillFTSSummary 在扫描后批量回填摘要，覆盖迁移重建或历史缺失的情况。
func (s *Store) BackfillFTSSummary(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE sessions_fts
SET summary = COALESCE((SELECT ai.summary FROM session_ai ai
                        WHERE ai.session_id = sessions_fts.session_id AND ai.status = 'ok'), '')
WHERE EXISTS (
  SELECT 1 FROM session_ai ai
  WHERE ai.session_id = sessions_fts.session_id AND ai.status = 'ok' AND ai.summary <> ''
)`)
	return err
}

func (s *Store) MarkDeleted(ctx context.Context, id string, now int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET state = 'deleted', deleted_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) MarkRestored(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET state = 'ok', deleted_at = 0 WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) HardDelete(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions_fts WHERE session_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CountSessions(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM sessions`).Scan(&n)
	return n, err
}

func (s *Store) CountFTS(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM sessions_fts`).Scan(&n)
	return n, err
}

// ResetFileState 让下一次扫描强制重新解析所有会话，用于首次建立全文索引。
func (s *Store) ResetFileState(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET file_mtime = -1`)
	return err
}
