// Author: zhangweiwei
package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"claude-session/internal/model"
)

const summarySystem = "你是会话归档助手。只输出 JSON，不要输出任何解释文字。"

func summarySchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "summary": {"type": "string"},
    "topics": {"type": "array", "items": {"type": "string"}},
    "title": {"type": "string"},
    "confidence": {"type": "number"},
    "drift": {"type": "number"}
  },
  "required": ["summary", "topics", "title", "confidence", "drift"],
  "additionalProperties": false
}`)
}

func titleSource(s model.Session) string {
	switch {
	case s.CustomTitle != "":
		return "用户自定义"
	case s.AITitle != "":
		return "Claude Code 自动生成"
	case s.LastPrompt != "":
		return "最后一条提问"
	default:
		return "无"
	}
}

func buildSummaryRequest(sess model.Session, projectName, sample, parentSummary string) Request {
	var b strings.Builder
	b.WriteString("以下是 Claude Code 会话的节选。请提取概要。\n\n")
	b.WriteString("[标题] ")
	b.WriteString(nonEmpty(sess.Title, "（无）"))
	b.WriteString("（来源：")
	b.WriteString(titleSource(sess))
	b.WriteString("）\n")
	b.WriteString("[项目] ")
	b.WriteString(nonEmpty(projectName, "（未知）"))
	b.WriteString("\n")
	if sess.GitBranch != "" {
		b.WriteString("[分支] ")
		b.WriteString(sess.GitBranch)
		b.WriteString("\n")
	}
	if parentSummary != "" {
		b.WriteString("[父会话概要] ")
		b.WriteString(truncate(parentSummary, 400))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(sample)
	b.WriteString("\n\n输出要求：\n")
	b.WriteString("- summary：80~150 字中文。写清「要解决什么问题、做了什么、结论是什么」。\n")
	b.WriteString("- 禁止：复述文件或工具清单；使用「用户与助手进行了讨论」这类空话；罗列报错原文超过一句。\n")
	b.WriteString("- topics：3~6 个名词短语，每个 2~8 字，用于归类检索。\n")
	b.WriteString("- title：10~25 字中文标题，能替代现有标题，概括会话主线。不要以「关于」「使用」开头，不要带分支名或日期。\n")
	b.WriteString("- confidence：0~1，表示对概要准确性的自评。\n")
	if parentSummary != "" {
		b.WriteString("- 这是从父会话分叉出来的会话。summary 要结合父会话概要，说明分叉后新增了什么，不要重复父会话已经完成的部分。\n")
		b.WriteString("- drift：0~1，表示分叉后主题相对父会话的偏移程度。0 表示仍在同一主题，1 表示完全转向。\n")
	} else {
		b.WriteString("- drift：固定输出 0。\n")
	}

	return Request{System: summarySystem, User: b.String(), Schema: summarySchema()}
}

type summaryPayload struct {
	Summary    string   `json:"summary"`
	Topics     []string `json:"topics"`
	Title      string   `json:"title"`
	Confidence float64  `json:"confidence"`
	Drift      float64  `json:"drift"`
}

func parseSummary(res Result) (summaryPayload, error) {
	raw := res.Structured
	if len(raw) == 0 {
		raw = json.RawMessage(extractJSONObject(res.Text))
	}
	if len(raw) == 0 {
		return summaryPayload{}, errors.New("模型未返回可解析的概要")
	}
	var out summaryPayload
	if err := json.Unmarshal(raw, &out); err != nil {
		return summaryPayload{}, errors.New("解析概要失败: " + err.Error())
	}
	if strings.TrimSpace(out.Summary) == "" {
		return summaryPayload{}, errors.New("模型返回的概要为空")
	}
	return out, nil
}

// --- 批量概要 ---

type BatchSummaryItem struct {
	Session       model.Session
	Project       string
	Sample        string
	ParentSummary string
}

func batchSummarySchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "items": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "session_id": {"type": "string"},
          "summary": {"type": "string"},
          "topics": {"type": "array", "items": {"type": "string"}},
          "title": {"type": "string"},
          "confidence": {"type": "number"},
          "drift": {"type": "number"}
        },
        "required": ["session_id", "summary", "topics", "title", "confidence", "drift"],
        "additionalProperties": false
      }
    }
  },
  "required": ["items"],
  "additionalProperties": false
}`)
}

func buildBatchSummaryRequest(items []BatchSummaryItem) Request {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("以下是 %d 个 Claude Code 会话的节选。请为每个会话提取概要。\n\n", len(items)))
	for i, item := range items {
		b.WriteString(fmt.Sprintf("[会话 %d] session_id=%s\n", i+1, item.Session.ID))
		b.WriteString("[标题] ")
		b.WriteString(nonEmpty(item.Session.Title, "（无）"))
		b.WriteString("（来源：")
		b.WriteString(titleSource(item.Session))
		b.WriteString("）\n")
		b.WriteString("[项目] ")
		b.WriteString(nonEmpty(item.Project, "（未知）"))
		b.WriteString("\n")
		if item.Session.GitBranch != "" {
			b.WriteString("[分支] ")
			b.WriteString(item.Session.GitBranch)
			b.WriteString("\n")
		}
		if item.ParentSummary != "" {
			b.WriteString("[父会话概要] ")
			b.WriteString(truncate(item.ParentSummary, 400))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(item.Sample)
		b.WriteString("\n\n")
	}

	b.WriteString("输出要求：\n")
	b.WriteString("- 输出 JSON 对象，items 是数组，每个输入会话对应一项，session_id 必须与输入完全一致，不要遗漏。\n")
	b.WriteString("- summary：80~150 字中文，写清「要解决什么问题、做了什么、结论是什么」。\n")
	b.WriteString("- 禁止：复述文件或工具清单；使用「用户与助手进行了讨论」这类空话；罗列报错原文超过一句。\n")
	b.WriteString("- topics：3~6 个名词短语，每个 2~8 字。\n")
	b.WriteString("- title：10~25 字中文标题，不以「关于」「使用」开头，不带分支名或日期。\n")
	b.WriteString("- confidence：0~1，对该会话概要准确性的自评。\n")
	b.WriteString("- drift：有父会话时表示分叉后的主题偏移（0~1）；否则固定 0。\n")

	return Request{System: summarySystem, User: b.String(), Schema: batchSummarySchema()}
}

func parseBatchSummary(res Result) (map[string]summaryPayload, error) {
	raw := res.Structured
	if len(raw) == 0 {
		raw = json.RawMessage(extractJSONObject(res.Text))
	}
	if len(raw) == 0 {
		return nil, errors.New("模型未返回可解析的批量概要")
	}
	var out struct {
		Items []struct {
			SessionID  string   `json:"session_id"`
			Summary    string   `json:"summary"`
			Topics     []string `json:"topics"`
			Title      string   `json:"title"`
			Confidence float64  `json:"confidence"`
			Drift      float64  `json:"drift"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, errors.New("解析批量概要失败: " + err.Error())
	}
	result := make(map[string]summaryPayload, len(out.Items))
	for _, item := range out.Items {
		if item.SessionID == "" || strings.TrimSpace(item.Summary) == "" {
			continue
		}
		result[item.SessionID] = summaryPayload{
			Summary: item.Summary, Topics: item.Topics, Title: item.Title,
			Confidence: item.Confidence, Drift: item.Drift,
		}
	}
	if len(result) == 0 {
		return nil, errors.New("批量概要结果为空")
	}
	return result, nil
}

func extractJSONObject(text string) string {
	start := strings.Index(text, "{")
	if start < 0 {
		return ""
	}
	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return ""
}

// --- 项目级归类 ---

type FolderNode struct {
	ID           int64
	Ref          int // 1-based 编号，供模型引用
	Path         []string
	Source       string
	Description  string
	SessionCount int
}

type CategorizeItem struct {
	SessionID    string
	Title        string
	TitleSource  string
	Summary      string
	Topics       []string
	ParentFolder []string
	LineageGroup string
	Drift        float64
}

type FolderSuggestionPayload struct {
	Name        string  `json:"name"`
	ParentRef   int     `json:"parent_ref"`
	ParentName  string  `json:"parent_name"`
	Description string  `json:"description"`
	Confidence  float64 `json:"confidence"`
}

type AssignmentPayload struct {
	SessionID  string  `json:"session_id"`
	Ref        int     `json:"ref"`
	NewFolder  string  `json:"new_folder"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

type CategorizePayload struct {
	Folders     []FolderSuggestionPayload `json:"folders"`
	Assignments []AssignmentPayload       `json:"assignments"`
}

func categorizeSchema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "folders": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "parent_ref": {"type": "integer"},
          "parent_name": {"type": "string"},
          "description": {"type": "string"},
          "confidence": {"type": "number"}
        },
        "required": ["name", "parent_ref", "parent_name", "description", "confidence"],
        "additionalProperties": false
      }
    },
    "assignments": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "session_id": {"type": "string"},
          "ref": {"type": "integer"},
          "new_folder": {"type": "string"},
          "confidence": {"type": "number"},
          "reason": {"type": "string"}
        },
        "required": ["session_id", "ref", "new_folder", "confidence", "reason"],
        "additionalProperties": false
      }
    }
  },
  "required": ["folders", "assignments"],
  "additionalProperties": false
}`)
}

func buildCategorizeRequest(projectName string, folders []FolderNode, items []CategorizeItem) Request {
	var b strings.Builder
	b.WriteString("以下是项目内待归类的会话。请设计归类并把会话分配到合适路径。\n\n")
	b.WriteString("项目：")
	b.WriteString(nonEmpty(projectName, "（未知）"))
	b.WriteString("\n\n")

	var userFolders, aiFolders []string
	for _, f := range folders {
		line := fmt.Sprintf("[%d] %s", f.Ref, strings.Join(f.Path, " / "))
		if f.Description != "" {
			line += "（" + f.Description + "）"
		}
		if f.Source == "user" {
			userFolders = append(userFolders, line)
		} else {
			aiFolders = append(aiFolders, line)
		}
	}
	if len(userFolders) > 0 {
		b.WriteString("已有归类（用户创建，不可改名或删除，可复用或作为父级）：\n")
		b.WriteString(strings.Join(userFolders, "\n"))
		b.WriteString("\n\n")
	}
	if len(aiFolders) > 0 {
		b.WriteString("已有归类（AI 创建，可复用或作为父级）：\n")
		b.WriteString(strings.Join(aiFolders, "\n"))
		b.WriteString("\n\n")
	}

	b.WriteString("待归类会话：\n")
	for _, item := range items {
		b.WriteString("- ")
		b.WriteString(item.SessionID)
		b.WriteString(" | 标题：")
		b.WriteString(nonEmpty(item.Title, "（无）"))
		b.WriteString("（来源：")
		b.WriteString(nonEmpty(item.TitleSource, "未知"))
		b.WriteString("）")
		if item.Summary != "" {
			b.WriteString(" | 概要：")
			b.WriteString(truncate(item.Summary, 200))
		}
		if len(item.Topics) > 0 {
			b.WriteString(" | topics：")
			b.WriteString(strings.Join(item.Topics, "、"))
		}
		if len(item.ParentFolder) > 0 {
			b.WriteString(" | 父会话归类：")
			b.WriteString(strings.Join(item.ParentFolder, " / "))
		}
		if item.LineageGroup != "" {
			b.WriteString(" | 谱系组：")
			b.WriteString(item.LineageGroup)
		}
		if item.Drift > 0 {
			b.WriteString(" | 分叉后主题偏移：")
			b.WriteString(fmt.Sprintf("%.2f", item.Drift))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n输出要求：\n")
	b.WriteString("1) 先判断会话能否语义匹配上面某个已有归类（即使名称措辞不同，含义相同也算匹配）。\n")
	b.WriteString("   能匹配就填 assignments.ref = 对应编号，new_folder 留空，不要新建。\n")
	b.WriteString("2) 匹配不上但存在语义上合适的父级（例如已有「开发」，新主题「SpringBoot」应作为它的子级）：\n")
	b.WriteString("   在 folders 中新建该归类，parent_ref 填父级编号；assignments.new_folder 填新归类名称。\n")
	b.WriteString("3) 完全没有合适的已有归类时，可以自行创建一级和二级：\n")
	b.WriteString("   一级：parent_ref=0 且 parent_name 留空；二级：parent_name 填本批新建的一级名称。\n")
	b.WriteString("4) 新建一级归类最多 9 个，层级最多 2 层。只有确实超过 9 个时才需要合并到「其他」；未超过时不要创建名为「其他」的归类。\n")
	b.WriteString("5) 归类名 2~8 字名词短语，本批新建的名称必须唯一；新建归类至少要有 2 个会话。\n")
	b.WriteString("   某个主题只有 1 个会话时，复用最接近的已有归类，或保持未归类（ref=0 且 new_folder 留空），不要为它单独新建归类。\n")
	b.WriteString("6) folders 按创建顺序排列，父级必须先于引用它的子级出现。\n")
	b.WriteString("7) 无法判断时 ref=0 且 new_folder 留空，表示保持未归类。\n")
	b.WriteString("8) 用户自定义标题表示明确的归类意图，优先服从；prompt 截断出的标题只作参考。\n")
	b.WriteString("9) 带相同「谱系组」标签的会话必须分到同一个归类；即使分叉后主题不同，也必须同归类。\n")
	b.WriteString("   没有标签的独立会话不受此限制，可以按主题彼此合并，也可以与任意谱系组归入同一归类；不同谱系组之间没有必须分开的约束。\n")
	b.WriteString("10) confidence：0~1，表示对该会话归类的把握。\n")

	return Request{System: "你是会话归档助手。只输出 JSON，不要输出任何解释文字。", User: b.String(), Schema: categorizeSchema()}
}

func parseCategorize(res Result) (CategorizePayload, error) {
	raw := res.Structured
	if len(raw) == 0 {
		raw = json.RawMessage(extractJSONObject(res.Text))
	}
	if len(raw) == 0 {
		return CategorizePayload{}, errors.New("模型未返回可解析的归类结果")
	}
	var out CategorizePayload
	if err := json.Unmarshal(raw, &out); err != nil {
		return CategorizePayload{}, errors.New("解析归类结果失败: " + err.Error())
	}
	if out.Folders == nil {
		out.Folders = []FolderSuggestionPayload{}
	}
	if out.Assignments == nil {
		out.Assignments = []AssignmentPayload{}
	}
	return out, nil
}

func nonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
