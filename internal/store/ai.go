// Author: zhangweiwei
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"claude-session/internal/model"
)

func nowUnix() int64 { return time.Now().Unix() }

// GetAISummary 读取单条会话的 AI 概要。没有记录时 found=false。
func (s *Store) GetAISummary(ctx context.Context, sessionID string) (model.AISummary, bool, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT session_id, summary, suggested_title, topics, model, profile, status, error,
       confidence, lineage_mode, drift, source_mtime, source_size,
       in_tokens, out_tokens, cost_usd, attempts, updated_at
FROM session_ai WHERE session_id = ?`, sessionID)
	a, err := scanAISummary(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.AISummary{}, false, nil
	}
	if err != nil {
		return model.AISummary{}, false, err
	}
	return a, true, nil
}

// ListAISummaries 批量读取概要，供列表页附加展示字段。
func (s *Store) ListAISummaries(ctx context.Context, ids []string) (map[string]model.AISummary, error) {
	out := make(map[string]model.AISummary, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT session_id, summary, suggested_title, topics, model, profile, status, error,
       confidence, lineage_mode, drift, source_mtime, source_size,
       in_tokens, out_tokens, cost_usd, attempts, updated_at
FROM session_ai WHERE session_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		a, err := scanAISummary(rows)
		if err != nil {
			return nil, err
		}
		out[a.SessionID] = a
	}
	return out, rows.Err()
}

func scanAISummary(row scanner) (model.AISummary, error) {
	var a model.AISummary
	var topics string
	err := row.Scan(
		&a.SessionID, &a.Summary, &a.SuggestedTitle, &topics, &a.Model, &a.Profile,
		&a.Status, &a.Error, &a.Confidence, &a.LineageMode, &a.Drift,
		&a.SourceMtime, &a.SourceSize, &a.InTokens, &a.OutTokens, &a.CostUSD,
		&a.Attempts, &a.UpdatedAt,
	)
	if err != nil {
		return model.AISummary{}, err
	}
	if topics != "" {
		_ = json.Unmarshal([]byte(topics), &a.Topics)
	}
	if a.Topics == nil {
		a.Topics = []string{}
	}
	return a, nil
}

// UpsertAISummary 写入完整概要。仅在生成成功时调用，失败路径走 MarkAISummaryFailed 保留旧值。
func (s *Store) UpsertAISummary(ctx context.Context, a model.AISummary) error {
	topics, err := json.Marshal(a.Topics)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO session_ai(
  session_id, summary, suggested_title, topics, model, profile, status, error,
  confidence, lineage_mode, drift, source_mtime, source_size,
  in_tokens, out_tokens, cost_usd, attempts, updated_at
) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(session_id) DO UPDATE SET
  summary = excluded.summary,
  suggested_title = excluded.suggested_title,
  topics = excluded.topics,
  model = excluded.model,
  profile = excluded.profile,
  status = excluded.status,
  error = excluded.error,
  confidence = excluded.confidence,
  lineage_mode = excluded.lineage_mode,
  drift = excluded.drift,
  source_mtime = excluded.source_mtime,
  source_size = excluded.source_size,
  in_tokens = excluded.in_tokens,
  out_tokens = excluded.out_tokens,
  cost_usd = excluded.cost_usd,
  attempts = excluded.attempts,
  updated_at = excluded.updated_at`,
		a.SessionID, a.Summary, a.SuggestedTitle, string(topics), a.Model, a.Profile,
		a.Status, a.Error, a.Confidence, a.LineageMode, a.Drift,
		a.SourceMtime, a.SourceSize, a.InTokens, a.OutTokens, a.CostUSD,
		a.Attempts, a.UpdatedAt)
	return err
}

// MarkAISummaryRunning 把会话标记为生成中，避免重复入队。
func (s *Store) MarkAISummaryRunning(ctx context.Context, sessionID string, attempts int) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO session_ai(session_id, status, attempts, updated_at) VALUES(?, 'running', ?, ?)
ON CONFLICT(session_id) DO UPDATE SET status = 'running', attempts = excluded.attempts, updated_at = excluded.updated_at`,
		sessionID, attempts, nowUnix())
	return err
}

// MarkAISummaryFailed 只更新状态与错误，保留上一次成功的摘要与建议标题。
func (s *Store) MarkAISummaryFailed(ctx context.Context, sessionID, errMsg string, attempts int) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE session_ai SET status = 'failed', error = ?, attempts = ?, updated_at = ?
WHERE session_id = ?`, errMsg, attempts, nowUnix(), sessionID)
	return err
}

// MarkAISummarySettled 在未取得结构化结果但需要落状态时使用。
func (s *Store) MarkAISummarySettled(ctx context.Context, sessionID, status, errMsg string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE session_ai SET status = ?, error = ?, updated_at = ? WHERE session_id = ?`,
		status, errMsg, nowUnix(), sessionID)
	return err
}

// PendingSummarySessions 返回当前项目下需要生成概要的会话。
// 项目口径与列表一致：使用 COALESCE(display_project_id, project_id)。
// skipFresh 为 true 时绕过「文件 60 秒内仍有写入」的保护，供用户手动触发的整理使用。
func (s *Store) PendingSummarySessions(ctx context.Context, projectID int64, minMessages, limit int, force, skipFresh bool) ([]model.Session, error) {
	now := nowUnix()
	forceFlag := 0
	if force {
		forceFlag = 1
	}
	freshFlag := 0
	if skipFresh {
		freshFlag = 1
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT `+sessionColumns+`
FROM sessions s
LEFT JOIN session_ai a ON a.session_id = s.id
JOIN projects p ON p.id = s.project_id
WHERE COALESCE(s.display_project_id, s.project_id) = ?
  AND s.state = 'ok'
  AND s.deleted_at = 0
  AND s.message_count >= ?
  AND s.file_mtime > 0
  AND (? = 1 OR s.file_mtime < ?)
  AND (a.session_id IS NULL OR a.source_mtime != s.file_mtime OR a.source_size != s.file_size OR ? = 1)
  AND (a.status IS NULL OR a.status NOT IN ('running', 'pending') OR ? = 1)
ORDER BY s.last_ts DESC, s.id ASC
LIMIT ?`, projectID, minMessages, freshFlag, now-60, forceFlag, forceFlag, limit)
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

// CountPendingSummaries 统计当前项目待生成的会话数，用于侧栏提示。
func (s *Store) CountPendingSummaries(ctx context.Context, projectID int64, minMessages int) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(1)
FROM sessions s
LEFT JOIN session_ai a ON a.session_id = s.id
WHERE COALESCE(s.display_project_id, s.project_id) = ?
  AND s.state = 'ok'
  AND s.deleted_at = 0
  AND s.message_count >= ?
  AND s.file_mtime > 0
  AND (a.session_id IS NULL OR a.source_mtime != s.file_mtime OR a.source_size != s.file_size)
  AND (a.status IS NULL OR a.status NOT IN ('running', 'pending'))`,
		projectID, minMessages).Scan(&n)
	return n, err
}

// GetSessionsByIDs 按 ID 批量取会话，用于详情页单条生成。
func (s *Store) GetSessionsByIDs(ctx context.Context, ids []string) ([]model.Session, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT `+sessionColumnsWithParent+`
FROM sessions s
LEFT JOIN sessions ps ON ps.id = s.parent_id
JOIN projects p ON p.id = s.project_id
WHERE s.id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.Session, 0, len(ids))
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

// attachAISummaries 给会话列表补充概要字段，避免各查询重复 JOIN。
func (s *Store) attachAISummaries(ctx context.Context, list []model.Session) {
	if len(list) == 0 {
		return
	}
	ids := make([]string, 0, len(list))
	for _, sess := range list {
		ids = append(ids, sess.ID)
	}
	m, err := s.ListAISummaries(ctx, ids)
	if err != nil {
		return
	}
	for i := range list {
		a, ok := m[list[i].ID]
		if !ok {
			continue
		}
		list[i].AISummary = a.Summary
		list[i].AISuggestedTitle = a.SuggestedTitle
		list[i].AIStatus = a.Status
		list[i].AIConfidence = a.Confidence
		list[i].AICanApplyTitle = list[i].CustomTitle == "" && a.SuggestedTitle != ""
	}
}

// --- settings ---

func (s *Store) GetSetting(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO settings(key, value) VALUES(?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// --- jobs ---

func (s *Store) CreateAIJob(ctx context.Context, j model.AIJob) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO ai_jobs(id, kind, project_id, status, total, done, failed, error, created_at, updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?)`,
		j.ID, j.Kind, j.ProjectID, j.Status, j.Total, j.Done, j.Failed, j.Error, j.CreatedAt, j.UpdatedAt)
	return err
}

func (s *Store) UpdateAIJob(ctx context.Context, id string, done, failed int, status, errMsg string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE ai_jobs SET done = ?, failed = ?, status = ?, error = ?, updated_at = ? WHERE id = ?`,
		done, failed, status, errMsg, nowUnix(), id)
	return err
}

func (s *Store) GetAIJob(ctx context.Context, id string) (model.AIJob, error) {
	var j model.AIJob
	err := s.db.QueryRowContext(ctx, `
SELECT id, kind, project_id, status, total, done, failed, error, created_at, updated_at
FROM ai_jobs WHERE id = ?`, id).Scan(
		&j.ID, &j.Kind, &j.ProjectID, &j.Status, &j.Total, &j.Done, &j.Failed,
		&j.Error, &j.CreatedAt, &j.UpdatedAt)
	return j, err
}

// LatestAIJob 返回同类型、同项目下最近的一条任务，供前端展示进度。
func (s *Store) LatestAIJob(ctx context.Context, kind string, projectID int64) (model.AIJob, error) {
	var j model.AIJob
	err := s.db.QueryRowContext(ctx, `
SELECT id, kind, project_id, status, total, done, failed, error, created_at, updated_at
FROM ai_jobs WHERE kind = ? AND project_id = ?
ORDER BY created_at DESC LIMIT 1`, kind, projectID).Scan(
		&j.ID, &j.Kind, &j.ProjectID, &j.Status, &j.Total, &j.Done, &j.Failed,
		&j.Error, &j.CreatedAt, &j.UpdatedAt)
	return j, err
}

// FindActiveJob 返回同类型、同项目下仍在排队的任务，用于幂等复用。
func (s *Store) FindActiveJob(ctx context.Context, kind string, projectID int64) (string, bool, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `
SELECT id FROM ai_jobs
WHERE kind = ? AND project_id = ? AND status IN ('queued', 'running')
ORDER BY created_at DESC LIMIT 1`, kind, projectID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

// ResetRunningSummaries 在服务重启后把中断的任务恢复为待处理。
func (s *Store) ResetRunningSummaries(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE session_ai SET status = 'pending' WHERE status = 'running'`)
	return err
}

// ResetActiveJobs 把上次进程遗留的 queued/running 任务置为失败。
// 否则 FindActiveJob 会把这些永远不会结束的任务复用给新请求。
func (s *Store) ResetActiveJobs(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE ai_jobs SET status = 'failed', error = '服务重启导致任务中断', updated_at = ?
WHERE status IN ('queued', 'running')`, nowUnix())
	return err
}

func (s *Store) MarkAIJobRunning(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE ai_jobs SET status = 'running', updated_at = ? WHERE id = ? AND status = 'queued'`, nowUnix(), id)
	return err
}

func (s *Store) IncAIJob(ctx context.Context, id string, failed bool) error {
	col := "done"
	if failed {
		col = "failed"
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE ai_jobs SET `+col+` = `+col+` + 1, updated_at = ? WHERE id = ?`, nowUnix(), id)
	return err
}

func (s *Store) FinishAIJobIfComplete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE ai_jobs SET status = 'done', updated_at = ?
WHERE id = ? AND done + failed >= total AND status IN ('queued', 'running')`, nowUnix(), id)
	return err
}

func (s *Store) FailAIJob(ctx context.Context, id, msg string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE ai_jobs SET status = 'failed', error = ?, updated_at = ? WHERE id = ?`, msg, nowUnix(), id)
	return err
}

// AITotals 汇总本地 AI 用量，用于设置页展示。
func (s *Store) AITotals(ctx context.Context) (count, failed, inTokens, outTokens int, cost float64, err error) {
	err = s.db.QueryRowContext(ctx, `
SELECT COUNT(1),
       COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0),
       COALESCE(SUM(in_tokens), 0),
       COALESCE(SUM(out_tokens), 0),
       COALESCE(SUM(cost_usd), 0)
FROM session_ai`).Scan(&count, &failed, &inTokens, &outTokens, &cost)
	return
}

// --- 归类建议 ---

type FolderSuggestion struct {
	ID          int64    `json:"id"`
	JobID       string   `json:"job_id"`
	ParentID    int64    `json:"parent_id"`   // 挂到已有归类
	ParentName  string   `json:"parent_name"` // 挂到本批新建的归类
	ParentPath  []string `json:"parent_path"` // 展示与旧数据兼容
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Confidence  float64  `json:"confidence"`
	Applied     bool     `json:"applied"`
}

type SessionSuggestion struct {
	ID         int64    `json:"id"`
	JobID      string   `json:"job_id"`
	SessionID  string   `json:"session_id"`
	FolderID   int64    `json:"folder_id"`   // 复用已有归类
	NewFolder  string   `json:"new_folder"`  // 归入本批新建的归类
	FolderPath []string `json:"folder_path"` // 展示与旧数据兼容
	Action     string   `json:"action"`
	Confidence float64  `json:"confidence"`
	Reason     string   `json:"reason"`
	Applied    bool     `json:"applied"`
}

func (s *Store) CreateFolderSuggestions(ctx context.Context, list []FolderSuggestion) error {
	for _, item := range list {
		parent, _ := json.Marshal(item.ParentPath)
		if _, err := s.db.ExecContext(ctx, `
INSERT INTO ai_folder_suggestions(job_id, parent_id, parent_name, parent_path, name, description, confidence, applied)
VALUES(?,?,?,?,?,?,?,0)`, item.JobID, item.ParentID, item.ParentName, string(parent),
			item.Name, item.Description, item.Confidence); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreateSessionSuggestions(ctx context.Context, list []SessionSuggestion) error {
	for _, item := range list {
		path, _ := json.Marshal(item.FolderPath)
		if _, err := s.db.ExecContext(ctx, `
INSERT INTO ai_suggestions(job_id, session_id, folder_id, new_folder, folder_path, action, confidence, reason, applied)
VALUES(?,?,?,?,?,?,?,?,0)`, item.JobID, item.SessionID, item.FolderID, item.NewFolder,
			string(path), item.Action, item.Confidence, item.Reason); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListFolderSuggestions(ctx context.Context, jobID string) ([]FolderSuggestion, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, job_id, parent_id, parent_name, parent_path, name, description, confidence, applied
FROM ai_folder_suggestions WHERE job_id = ? ORDER BY id ASC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FolderSuggestion
	for rows.Next() {
		var item FolderSuggestion
		var parent string
		var applied int
		if err := rows.Scan(&item.ID, &item.JobID, &item.ParentID, &item.ParentName, &parent,
			&item.Name, &item.Description, &item.Confidence, &applied); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(parent), &item.ParentPath)
		item.Applied = applied == 1
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListSessionSuggestions(ctx context.Context, jobID string) ([]SessionSuggestion, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, job_id, session_id, folder_id, new_folder, folder_path, action, confidence, reason, applied
FROM ai_suggestions WHERE job_id = ? ORDER BY id ASC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionSuggestion
	for rows.Next() {
		var item SessionSuggestion
		var path string
		var applied int
		if err := rows.Scan(&item.ID, &item.JobID, &item.SessionID, &item.FolderID, &item.NewFolder,
			&path, &item.Action, &item.Confidence, &item.Reason, &applied); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(path), &item.FolderPath)
		item.Applied = applied == 1
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) MarkFolderSuggestionsApplied(ctx context.Context, ids []int64) error {
	for _, id := range ids {
		if _, err := s.db.ExecContext(ctx, `UPDATE ai_folder_suggestions SET applied = 1 WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) MarkSessionSuggestionsApplied(ctx context.Context, ids []int64) error {
	for _, id := range ids {
		if _, err := s.db.ExecContext(ctx, `UPDATE ai_suggestions SET applied = 1 WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteAISuggestions(ctx context.Context, jobID string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM ai_suggestions WHERE job_id = ?`, jobID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM ai_folder_suggestions WHERE job_id = ?`, jobID)
	return err
}

// CategorizeCandidates 返回参与归类聚类的会话：未归类，或在显式要求时包含已由 AI 归类的会话。
// 手动归类的会话永不参与。
func (s *Store) CategorizeCandidates(ctx context.Context, projectID int64, reevaluate bool) ([]model.Session, error) {
	flag := 0
	if reevaluate {
		flag = 1
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT `+sessionColumns+`
FROM sessions s
JOIN session_ai a ON a.session_id = s.id
JOIN projects p ON p.id = s.project_id
WHERE COALESCE(s.display_project_id, s.project_id) = ?
  AND s.state = 'ok'
  AND s.deleted_at = 0
  AND a.status = 'ok'
  AND a.summary <> ''
  AND (
        (s.folder_id IS NULL AND s.folder_source = '')
     OR (s.folder_source = 'ai' AND ? = 1)
  )
ORDER BY s.last_ts DESC, s.id ASC
LIMIT 300`, projectID, flag)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.attachAISummaries(ctx, out)
	return out, nil
}

// CountProjectSessions 统计当前展示口径下的有效会话数。
func (s *Store) CountProjectSessions(ctx context.Context, projectID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(1) FROM sessions s
WHERE COALESCE(s.display_project_id, s.project_id) = ?
  AND s.state = 'ok' AND s.deleted_at = 0`, projectID).Scan(&n)
	return n, err
}

// CountProjectSummaries 统计已有可用概要的会话数，用于解释归类候选为空的原因。
func (s *Store) CountProjectSummaries(ctx context.Context, projectID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(1) FROM sessions s
JOIN session_ai a ON a.session_id = s.id
WHERE COALESCE(s.display_project_id, s.project_id) = ?
  AND s.state = 'ok' AND s.deleted_at = 0
  AND a.status = 'ok' AND a.summary <> ''`, projectID).Scan(&n)
	return n, err
}

// SessionFolderMap 返回项目内会话到归类的映射，用于补充派生会话的父归类提示。
func (s *Store) SessionFolderMap(ctx context.Context, projectID int64) (map[string]*int64, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT s.id, s.folder_id FROM sessions s
WHERE COALESCE(s.display_project_id, s.project_id) = ?`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]*int64)
	for rows.Next() {
		var id string
		var folder sql.NullInt64
		if err := rows.Scan(&id, &folder); err != nil {
			return nil, err
		}
		if folder.Valid {
			v := folder.Int64
			out[id] = &v
		} else {
			out[id] = nil
		}
	}
	return out, rows.Err()
}

// SetSessionFolderAI 写入 AI 建议的归类，并标记来源，便于后续整理覆盖。
func (s *Store) SetSessionFolderAI(ctx context.Context, sessionID string, folderID int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET folder_id = ?, folder_source = 'ai' WHERE id = ?`, folderID, sessionID)
	return err
}

// CreateAIFolder 创建 AI 归类节点，source 固定为 ai。
func (s *Store) CreateAIFolder(ctx context.Context, projectID int64, name string, parentID *int64, description string, now int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
INSERT INTO folders(project_id, parent_id, name, sort_order, created_at, source, description)
VALUES(?,?,?,?,?, 'ai', ?)`, projectID, parentID, name, now, now, description)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// LatestActiveProject 返回最近打开且仍在活跃窗口内的项目，用于按需生成概要。
func (s *Store) LatestActiveProject(ctx context.Context, since int64) (model.Project, bool, error) {
	var p model.Project
	var alias string
	err := s.db.QueryRowContext(ctx, `
SELECT id, cwd, encoded_dir, name, alias, last_scan_at, pinned, last_opened_at
FROM projects WHERE last_opened_at >= ? AND last_opened_at > 0
ORDER BY last_opened_at DESC LIMIT 1`, since).Scan(
		&p.ID, &p.Cwd, &p.EncodedDir, &p.Name, &alias, &p.LastScanAt, &p.Pinned, &p.LastOpenedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Project{}, false, nil
	}
	if err != nil {
		return model.Project{}, false, err
	}
	p.Alias = alias
	return p, true, nil
}
