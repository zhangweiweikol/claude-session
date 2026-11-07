// Author: zhangweiwei
package model

// Project 对应一个 Claude Code 工作目录（jsonl 中 cwd 字段的去重结果）。
type Project struct {
	ID           int64  `json:"id"`
	Cwd          string `json:"cwd"`
	EncodedDir   string `json:"encoded_dir"`
	Name         string `json:"name"`
	Alias        string `json:"alias,omitempty"`
	OriginalName string `json:"original_name,omitempty"`
	SessionCount int    `json:"session_count"`
	LastScanAt   int64  `json:"last_scan_at"`
	Pinned       bool   `json:"pinned"`
	LastOpenedAt int64  `json:"last_opened_at"`
}

// Folder 是纯逻辑归类，只存在于本系统的 SQLite 中，不改变 Claude Code 的目录结构。
type Folder struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	ParentID  *int64 `json:"parent_id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

// Session 是一条 Claude Code 会话的索引记录，真值始终是对应的 jsonl 文件。
type Session struct {
	ID                   string `json:"id"`
	ProjectID            int64  `json:"project_id"`
	DisplayProjectID     *int64 `json:"display_project_id,omitempty"`
	DisplayProjectSource string `json:"display_project_source,omitempty"`
	FolderID             *int64 `json:"folder_id"`
	Title                string `json:"title"`
	AITitle              string `json:"ai_title,omitempty"`
	CustomTitle          string `json:"custom_title,omitempty"`
	LastPrompt           string `json:"last_prompt,omitempty"`
	MessageCount         int    `json:"message_count"`
	TokensInput          int    `json:"tokens_input"`
	TokensOutput         int    `json:"tokens_output"`
	FilePath             string `json:"-"`
	FileSize             int64  `json:"file_size"`
	FileMtime            int64  `json:"file_mtime"`
	FirstTS              string `json:"first_ts,omitempty"`
	LastTS               string `json:"last_ts,omitempty"`
	GitBranch            string `json:"git_branch,omitempty"`
	CCVersion            string `json:"cc_version,omitempty"`
	State                string `json:"state"`
	DeletedAt            int64  `json:"deleted_at,omitempty"`
	Snippet              string `json:"snippet,omitempty"`
	Active               bool   `json:"active"`
	Cwd                  string `json:"cwd,omitempty"`
	// 派生关系来自 jsonl 首行的 forkedFrom，parent_* 是联表查出的父会话摘要。
	ParentID      string `json:"parent_id,omitempty"`
	ForkPointUUID string `json:"fork_point_uuid,omitempty"`
	ParentTitle   string `json:"parent_title,omitempty"`
	ParentState   string `json:"parent_state,omitempty"`
}

type ToolCall struct {
	Name  string `json:"name"`
	Input string `json:"input"`
}

type Message struct {
	Type      string     `json:"type"`
	UUID      string     `json:"uuid"`
	Timestamp string     `json:"timestamp"`
	Text      string     `json:"text"`
	Tools     []ToolCall `json:"tools,omitempty"`
}

type SessionDetail struct {
	Session   Session   `json:"session"`
	Messages  []Message `json:"messages"`
	Truncated bool      `json:"truncated"`
}

// SessionFilter 用于列表查询。FolderID 为空且 Unfiled 为 false 时返回全部。
type SessionFilter struct {
	ProjectID   int64
	FolderID    *int64
	Unfiled     bool
	Categorized bool
	Query       string
	Deleted     bool
	Limit       int
}
