// Author: zhangweiwei
package scanner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"claude-session/internal/model"
	"claude-session/internal/store"
)

const (
	maxLineSize   = 16 << 20 // 单行最大 16MB，会话里存在较大的请求快照行
	maxLastPrompt = 300
	maxTitleLen   = 80
	maxMessages   = 300
	maxToolInput  = 400
	maxIndexBytes = 200 << 10
)

type Scanner struct {
	projectsDir string
	store       *store.Store
	mu          sync.Mutex
}

func New(projectsDir string, st *store.Store) *Scanner {
	return &Scanner{projectsDir: projectsDir, store: st}
}

type Result struct {
	Projects int           `json:"projects"`
	Sessions int           `json:"sessions"`
	Skipped  int           `json:"skipped"`
	Errors   []string      `json:"errors,omitempty"`
	Duration time.Duration `json:"-"`
}

type meta struct {
	ID           string
	Cwd          string
	LastCwd      string
	AITitle      string
	CustomTitle  string
	LastPrompt   string
	GitBranch    string
	CCVersion    string
	FirstTS      string
	LastTS       string
	MessageCount int
	TokensInput  int
	TokensOutput int
	Content      string
	ParentID     string
	ForkPoint    string
}

func (s *Scanner) Scan(ctx context.Context) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	start := time.Now()
	res := Result{}
	seen := make(map[string]int64) // 会话 ID -> 已处理文件的最新 mtime
	projects := make(map[string]bool)
	needFTSBackfill := false

	entries, err := os.ReadDir(s.projectsDir)
	if err != nil {
		return res, fmt.Errorf("读取会话目录失败: %w", err)
	}

	now := time.Now().Unix()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirPath := filepath.Join(s.projectsDir, entry.Name())
		files, err := filepath.Glob(filepath.Join(dirPath, "*.jsonl"))
		if err != nil {
			res.Errors = appendError(res.Errors, fmt.Sprintf("%s: %v", entry.Name(), err))
			continue
		}
		for _, file := range files {
			if ctx.Err() != nil {
				return res, ctx.Err()
			}
			id := strings.TrimSuffix(filepath.Base(file), ".jsonl")
			if !isSessionID(id) {
				res.Skipped++
				continue
			}
			info, err := os.Stat(file)
			if err != nil {
				res.Errors = appendError(res.Errors, fmt.Sprintf("%s: %v", id, err))
				continue
			}

			// 同一会话可能因 /cd 出现在多个项目目录中，保留 mtime 最新的那份。
			if prev, ok := seen[id]; ok && info.ModTime().Unix() <= prev {
				continue
			}

			if size, mtime, state, ok, err := s.store.FileState(ctx, id); err == nil && ok &&
				state == "ok" && size == info.Size() && mtime == info.ModTime().Unix() {
				seen[id] = info.ModTime().Unix()
				continue
			}

			m, err := parseMeta(file, id)
			if err != nil {
				res.Errors = appendError(res.Errors, fmt.Sprintf("%s: %v", id, err))
				continue
			}
			if m.Cwd == "" {
				res.Skipped++
				continue
			}

			pid, err := s.store.UpsertProject(ctx, m.Cwd, EncodeCwd(m.Cwd), projectName(m.Cwd), now)
			if err != nil {
				res.Errors = appendError(res.Errors, fmt.Sprintf("%s: %v", id, err))
				continue
			}
			projects[m.Cwd] = true

			var displayProjectID *int64
			if m.LastCwd != "" && !strings.EqualFold(filepath.Clean(m.LastCwd), filepath.Clean(m.Cwd)) {
				allowed, err := s.store.AutoDisplayAllowed(ctx, id)
				if err != nil {
					res.Errors = appendError(res.Errors, fmt.Sprintf("%s: 检查归属设置失败: %v", id, err))
				} else if allowed {
					lastPID, err := s.store.UpsertProject(ctx, m.LastCwd, EncodeCwd(m.LastCwd), projectName(m.LastCwd), now)
					if err != nil {
						res.Errors = appendError(res.Errors, fmt.Sprintf("%s: 记录归属目录失败: %v", id, err))
					} else {
						displayProjectID = &lastPID
						projects[m.LastCwd] = true
					}
				}
			}

			sess := model.Session{
				ID:            id,
				ProjectID:     pid,
				Title:         buildTitle(m.CustomTitle, m.AITitle, m.LastPrompt, id),
				AITitle:       m.AITitle,
				CustomTitle:   m.CustomTitle,
				LastPrompt:    truncate(m.LastPrompt, maxLastPrompt),
				MessageCount:  m.MessageCount,
				TokensInput:   m.TokensInput,
				TokensOutput:  m.TokensOutput,
				FilePath:      file,
				FileSize:      info.Size(),
				FileMtime:     info.ModTime().Unix(),
				FirstTS:       m.FirstTS,
				LastTS:        m.LastTS,
				GitBranch:     m.GitBranch,
				CCVersion:     m.CCVersion,
				State:         "ok",
				ParentID:      m.ParentID,
				ForkPointUUID: m.ForkPoint,
			}
			if err := s.store.UpsertSession(ctx, sess); err != nil {
				res.Errors = appendError(res.Errors, fmt.Sprintf("%s: %v", id, err))
				continue
			}
			// 派生会话继承父会话的归类；只填空值，不覆盖用户已手动调整的归类。
			if m.ParentID != "" {
				if folderID, found, err := s.store.SessionFolder(ctx, m.ParentID); err != nil {
					res.Errors = appendError(res.Errors, fmt.Sprintf("%s: 读取父会话归类失败: %v", id, err))
				} else if found && folderID != nil {
					if err := s.store.InheritFolderIfEmpty(ctx, id, *folderID); err != nil {
						res.Errors = appendError(res.Errors, fmt.Sprintf("%s: 继承父会话归类失败: %v", id, err))
					}
				}
			}
			if err := s.store.ApplyAutoDisplayProject(ctx, id, displayProjectID); err != nil {
				res.Errors = appendError(res.Errors, fmt.Sprintf("%s: 更新归属项目失败: %v", id, err))
			}
			if err := s.store.IndexSession(ctx, id, sess.Title, sess.LastPrompt, m.Content); err != nil {
				res.Errors = appendError(res.Errors, fmt.Sprintf("%s: 建立全文索引失败: %v", id, err))
			} else {
				needFTSBackfill = true
			}
			seen[id] = info.ModTime().Unix()
			res.Sessions++
		}
	}

	present := make(map[string]bool, len(seen))
	for id := range seen {
		present[id] = true
	}
	if err := s.store.MarkMissing(ctx, present); err != nil {
		return res, err
	}
	// 重建或新增索引后回填已有概要，保证搜索能命中摘要文本。
	if needFTSBackfill {
		if err := s.store.BackfillFTSSummary(ctx); err != nil {
			res.Errors = appendError(res.Errors, "回填概要索引失败: "+err.Error())
		}
	}
	res.Sessions = len(present)
	res.Projects = len(projects)
	res.Duration = time.Since(start)
	return res, nil
}

func parseMeta(path, id string) (meta, error) {
	f, err := os.Open(path)
	if err != nil {
		return meta{}, err
	}
	defer f.Close()

	var m meta
	m.ID = id
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 256<<10), maxLineSize)

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var head struct {
			Type       string `json:"type"`
			ForkedFrom *struct {
				SessionID   string `json:"sessionId"`
				MessageUUID string `json:"messageUuid"`
			} `json:"forkedFrom"`
		}
		if json.Unmarshal(line, &head) != nil {
			continue
		}
		// branch/fork 生成的会话在首行写入 forkedFrom，记录父会话与分叉点。
		if m.ParentID == "" && head.ForkedFrom != nil && isSessionID(head.ForkedFrom.SessionID) {
			m.ParentID = head.ForkedFrom.SessionID
			m.ForkPoint = head.ForkedFrom.MessageUUID
		}
		switch head.Type {
		case "user", "assistant":
			var row struct {
				IsSidechain bool            `json:"isSidechain"`
				Timestamp   string          `json:"timestamp"`
				Cwd         string          `json:"cwd"`
				GitBranch   string          `json:"gitBranch"`
				Version     string          `json:"version"`
				Message     json.RawMessage `json:"message"`
			}
			if json.Unmarshal(line, &row) != nil {
				continue
			}
			if row.Cwd != "" {
				if m.Cwd == "" {
					m.Cwd = row.Cwd
				}
				m.LastCwd = row.Cwd
			}
			if m.GitBranch == "" && row.GitBranch != "" {
				m.GitBranch = row.GitBranch
			}
			if m.CCVersion == "" && row.Version != "" {
				m.CCVersion = row.Version
			}
			if row.Timestamp != "" {
				if m.FirstTS == "" {
					m.FirstTS = row.Timestamp
				}
				m.LastTS = row.Timestamp
			}
			if !row.IsSidechain {
				if head.Type == "assistant" {
					var payload struct {
						Usage struct {
							InputTokens              int `json:"input_tokens"`
							OutputTokens             int `json:"output_tokens"`
							CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
							CacheReadInputTokens     int `json:"cache_read_input_tokens"`
						} `json:"usage"`
					}
					if json.Unmarshal(row.Message, &payload) == nil {
						m.TokensInput += payload.Usage.InputTokens +
							payload.Usage.CacheCreationInputTokens +
							payload.Usage.CacheReadInputTokens
						m.TokensOutput += payload.Usage.OutputTokens
					}
				}
				if isCountable(row.Message) {
					m.MessageCount++
				}
				if len(m.Content) < maxIndexBytes {
					if text := extractMessageText(row.Message); text != "" {
						m.Content += text + "\n"
						if len(m.Content) > maxIndexBytes {
							m.Content = strings.ToValidUTF8(m.Content[:maxIndexBytes], "")
						}
					}
				}
			}
		case "ai-title":
			var row struct {
				AITitle string `json:"aiTitle"`
			}
			if json.Unmarshal(line, &row) == nil && row.AITitle != "" {
				m.AITitle = row.AITitle
			}
		case "custom-title":
			var row struct {
				CustomTitle string `json:"customTitle"`
			}
			if json.Unmarshal(line, &row) == nil && row.CustomTitle != "" {
				m.CustomTitle = row.CustomTitle
			}
		case "last-prompt":
			var row struct {
				LastPrompt string `json:"lastPrompt"`
			}
			if json.Unmarshal(line, &row) == nil && row.LastPrompt != "" {
				m.LastPrompt = row.LastPrompt
			}
		}
	}
	return m, sc.Err()
}

// AppendCustomTitle 追加一条 custom-title 行，让 Claude Code 的 --resume 也能按新标题找到会话。
func AppendCustomTitle(path, sessionID, title string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	line, err := json.Marshal(struct {
		Type        string `json:"type"`
		CustomTitle string `json:"customTitle"`
		SessionID   string `json:"sessionId"`
	}{Type: "custom-title", CustomTitle: title, SessionID: sessionID})
	if err != nil {
		return err
	}
	line = append(line, '\n')
	_, err = f.Write(line)
	return err
}

// ReadMessages 从 jsonl 中提取可展示的对话，最多返回最近 maxMessages 条。
func ReadMessages(path string) ([]model.Message, bool, error) {
	return readMessages(path, maxMessages)
}

// ReadAllMessages 读取全部可展示消息，不做条数截断，用于导出。
func ReadAllMessages(path string) ([]model.Message, error) {
	msgs, _, err := readMessages(path, 0)
	return msgs, err
}

func readMessages(path string, limit int) ([]model.Message, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	var out []model.Message
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 256<<10), maxLineSize)

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var head struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(line, &head) != nil {
			continue
		}
		if head.Type != "user" && head.Type != "assistant" {
			continue
		}
		var row struct {
			Type        string `json:"type"`
			UUID        string `json:"uuid"`
			Timestamp   string `json:"timestamp"`
			IsSidechain bool   `json:"isSidechain"`
			Message     struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &row) != nil || row.IsSidechain {
			continue
		}
		text, tools := extractContent(row.Message.Content)
		if text == "" && len(tools) == 0 {
			continue
		}
		out = append(out, model.Message{
			Type:      row.Type,
			UUID:      row.UUID,
			Timestamp: row.Timestamp,
			Text:      text,
			Tools:     tools,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, false, err
	}

	if limit > 0 && len(out) > limit {
		return out[len(out)-limit:], true, nil
	}
	return out, false, nil
}

type block struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

func extractMessageText(raw json.RawMessage) string {
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &msg) != nil || len(msg.Content) == 0 {
		return ""
	}
	text, _ := extractContent(msg.Content)
	return text
}

func extractContent(raw json.RawMessage) (string, []model.ToolCall) {
	if len(raw) == 0 {
		return "", nil
	}
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return strings.TrimSpace(plain), nil
	}
	var blocks []block
	if json.Unmarshal(raw, &blocks) != nil {
		return "", nil
	}
	var texts []string
	var tools []model.ToolCall
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				texts = append(texts, strings.TrimSpace(b.Text))
			}
		case "tool_use":
			input := ""
			if len(b.Input) > 0 {
				input = truncate(string(b.Input), maxToolInput)
			}
			tools = append(tools, model.ToolCall{Name: b.Name, Input: input})
		}
	}
	return strings.Join(texts, "\n\n"), tools
}

func isCountable(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &msg) != nil || len(msg.Content) == 0 {
		return false
	}
	var plain string
	if json.Unmarshal(msg.Content, &plain) == nil {
		return strings.TrimSpace(plain) != ""
	}
	var blocks []block
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return false
	}
	for _, b := range blocks {
		if (b.Type == "text" && strings.TrimSpace(b.Text) != "") || b.Type == "tool_use" {
			return true
		}
	}
	return false
}

func isSessionID(name string) bool {
	if len(name) != 36 {
		return false
	}
	for i, c := range name {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// EncodeCwd 复刻 Claude Code 的项目目录命名规则：非字母数字字符替换为 '-'。
func EncodeCwd(cwd string) string {
	var b strings.Builder
	for _, r := range cwd {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func projectName(cwd string) string {
	clean := strings.TrimRight(cwd, `\/`)
	base := filepath.Base(clean)
	if base == "." || base == "" {
		return cwd
	}
	return base
}

func buildTitle(custom, ai, lastPrompt, id string) string {
	if custom != "" {
		return truncate(custom, maxTitleLen)
	}
	if ai != "" {
		return truncate(ai, maxTitleLen)
	}
	p := strings.Join(strings.Fields(lastPrompt), " ")
	if p != "" {
		return truncate(p, maxTitleLen)
	}
	return id
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

func appendError(errs []string, msg string) []string {
	if len(errs) >= 20 {
		return errs
	}
	return append(errs, msg)
}
