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
	ID          int64  `json:"id"`
	ProjectID   int64  `json:"project_id"`
	ParentID    *int64 `json:"parent_id"`
	Name        string `json:"name"`
	SortOrder   int    `json:"sort_order"`
	Source      string `json:"source,omitempty"`      // user|ai
	Description string `json:"description,omitempty"` // AI 生成的归类说明
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

	// AI 概要的列表摘要字段，由 store 批量附加，不参与 sessionColumns 查询。
	AISummary        string  `json:"ai_summary,omitempty"`
	AISuggestedTitle string  `json:"ai_suggested_title,omitempty"`
	AIStatus         string  `json:"ai_status,omitempty"`
	AIConfidence     float64 `json:"ai_confidence,omitempty"`
	AICanApplyTitle  bool    `json:"ai_can_apply_title,omitempty"`
}

// AISummary 是会话的模型摘要，1:1 对应 session_ai 表。
type AISummary struct {
	SessionID      string   `json:"session_id"`
	Summary        string   `json:"summary"`
	SuggestedTitle string   `json:"suggested_title,omitempty"`
	Topics         []string `json:"topics,omitempty"`
	Model          string   `json:"model,omitempty"`
	Profile        string   `json:"profile,omitempty"`
	Status         string   `json:"status"`
	Error          string   `json:"error,omitempty"`
	Confidence     float64  `json:"confidence,omitempty"`
	LineageMode    string   `json:"lineage_mode,omitempty"`
	Drift          float64  `json:"drift,omitempty"`
	InTokens       int      `json:"in_tokens,omitempty"`
	OutTokens      int      `json:"out_tokens,omitempty"`
	CostUSD        float64  `json:"cost_usd,omitempty"`
	Attempts       int      `json:"attempts,omitempty"`
	UpdatedAt      int64    `json:"updated_at,omitempty"`

	// 内容指纹与 lineage 信息只用于内部增量判断，不输出给前端。
	SourceMtime int64 `json:"-"`
	SourceSize  int64 `json:"-"`
}

// AIJob 是一次批量 AI 任务的状态记录。
type AIJob struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	ProjectID int64  `json:"project_id,omitempty"`
	Status    string `json:"status"`
	Total     int    `json:"total"`
	Done      int    `json:"done"`
	Failed    int    `json:"failed"`
	Error     string `json:"error,omitempty"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// AISettings 是 AI 功能的本地配置，存于 settings 表（ai.* 前缀）。
type AISettings struct {
	Enabled               bool     `json:"enabled"`
	Model                 string   `json:"model"`
	Profile               string   `json:"profile"`
	MinMessages           int      `json:"min_messages"`
	MaxChars              int      `json:"max_chars"`
	IncludeConclusionTail bool     `json:"include_conclusion_tail"`
	AutoEnqueueLimit      int      `json:"auto_enqueue_limit"`
	SummaryBatchSize      int      `json:"summary_batch_size"`
	AutoCategorize        bool     `json:"auto_categorize"`
	ExcludeProjects       []string `json:"exclude_projects"`
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
	Session   Session    `json:"session"`
	Messages  []Message  `json:"messages"`
	Truncated bool       `json:"truncated"`
	AI        *AISummary `json:"ai,omitempty"`
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
