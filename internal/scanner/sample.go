// Author: zhangweiwei
package scanner

import (
	"bufio"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	sampleHeadUsers   = 8
	sampleTailUsers   = 4
	sampleConclRunes  = 400
	sampleFirstRunes  = 150
	sampleAckMaxRunes = 8
)

// 无信息量的确认类短句，不进入概要输入。
var ackPattern = regexp.MustCompile(`^(继续|好的|好|可以|嗯+|是的|对|不对|再试|重试|ok|okay|yes|no|go on|继续吧|稍等|谢谢)[。.!！?？\s]*$`)

var redactRules = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`), "[REDACTED_KEY]"},
	{regexp.MustCompile(`ghp_[A-Za-z0-9]{20,}`), "[REDACTED_TOKEN]"},
	{regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "[REDACTED_KEY]"},
	{regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._\-]+`), "Bearer [REDACTED]"},
	{regexp.MustCompile(`(?i)(password|passwd|secret)\s*[=:]\s*\S+`), "$1=[REDACTED]"},
}

// SummarySample 是概要输入及其增量统计。
type SummarySample struct {
	Text              string
	IncrementMessages int  // 分叉后的有效消息数
	IncrementRunes    int  // 分叉后的正文字符数
	ForkFound         bool // 是否定位到分叉点；forkUUID 为空时视为不限
}

// SampleForSummary 流式读取 jsonl，按「前 8 + 后 4 条有效用户提问 + 结论段」
// 生成概要输入。内存占用与会话文件大小无关。
func SampleForSummary(path string, budget int, includeTail bool) (string, error) {
	s, err := SampleForSummaryFrom(path, "", budget, includeTail)
	return s.Text, err
}

// SampleForSummaryFrom 只采样分叉点之后的内容，用于派生会话的增量摘要。
// forkUUID 为空时等价于全量采样。
func SampleForSummaryFrom(path, forkUUID string, budget int, includeTail bool) (SummarySample, error) {
	f, err := os.Open(path)
	if err != nil {
		return SummarySample{}, err
	}
	defer f.Close()

	if budget <= 0 {
		budget = 1500
	}

	type part struct {
		label    string
		text     string
		priority int // 1 最高：后段，2：前段，3：结论段
		order    int
	}

	var head []string
	var tail [sampleTailUsers]string
	tailWrites := 0
	var firstAssistant, lastAssistant string
	order := 0
	reachedFork := forkUUID == ""
	incMessages := 0
	incRunes := 0

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 256<<10), maxLineSize)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var row struct {
			Type         string `json:"type"`
			UUID         string `json:"uuid"`
			IsSidechain  bool   `json:"isSidechain"`
			PromptSource string `json:"promptSource"`
			Origin       *struct {
				Kind string `json:"kind"`
			} `json:"origin"`
			Message json.RawMessage `json:"message"`
		}
		if json.Unmarshal(line, &row) != nil {
			continue
		}
		if !reachedFork {
			// 分叉点本身属于父会话内容，从它的下一行开始才算增量。
			if row.UUID == forkUUID {
				reachedFork = true
			}
			continue
		}
		if row.Type != "user" && row.Type != "assistant" {
			continue
		}
		if row.IsSidechain {
			continue
		}

		text, hasToolResult := sampleMessageText(row.Message)
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}

		if row.Type == "user" {
			// 工具结果同样记录为 user 消息，必须剔除，否则输入全是命令输出。
			if hasToolResult {
				continue
			}
			// 老版本 jsonl 可能没有 origin/promptSource，此时退回文本过滤。
			if row.Origin != nil && row.Origin.Kind != "" && row.Origin.Kind != "human" {
				continue
			}
			if row.PromptSource != "" && row.PromptSource != "typed" {
				continue
			}
			if utf8.RuneCountInString(text) <= sampleAckMaxRunes && ackPattern.MatchString(text) {
				continue
			}
			incMessages++
			incRunes += utf8.RuneCountInString(text)
			if len(head) < sampleHeadUsers {
				head = append(head, text)
			} else {
				tail[tailWrites%sampleTailUsers] = text
				tailWrites++
			}
			continue
		}

		incMessages++
		incRunes += utf8.RuneCountInString(text)
		if firstAssistant == "" {
			firstAssistant = text
		}
		lastAssistant = text
	}
	if err := sc.Err(); err != nil {
		return SummarySample{}, err
	}

	parts := make([]part, 0, sampleHeadUsers+sampleTailUsers+2)
	for _, t := range head {
		parts = append(parts, part{label: "用户", text: redact(t), priority: 2, order: order})
		order++
	}

	n := tailWrites
	if n > sampleTailUsers {
		n = sampleTailUsers
	}
	start := tailWrites - n
	for i := 0; i < n; i++ {
		parts = append(parts, part{
			label: "用户", text: redact(tail[(start+i)%sampleTailUsers]),
			priority: 1, order: order,
		})
		order++
	}

	if includeTail && lastAssistant != "" {
		parts = append(parts, part{label: "结论", text: redact(truncateRunes(lastAssistant, sampleConclRunes)), priority: 3, order: order})
		order++
		// 用户提问很短时补一个开头上下文，避免只有一句问题无法判断主题。
		if len(head) > 0 && totalRunes(head) < 300 && firstAssistant != "" {
			parts = append(parts, part{label: "开头", text: redact(truncateRunes(firstAssistant, sampleFirstRunes)), priority: 4, order: order})
			order++
		}
	}

	// 预算分配：后段 > 前段 > 结论段 > 开头。
	byPriority := make([]part, len(parts))
	copy(byPriority, parts)
	for i := 0; i < len(byPriority); i++ {
		for j := i + 1; j < len(byPriority); j++ {
			if byPriority[j].priority < byPriority[i].priority {
				byPriority[i], byPriority[j] = byPriority[j], byPriority[i]
			}
		}
	}

	keep := make(map[int]bool, len(parts))
	used := 0
	for _, p := range byPriority {
		cost := utf8.RuneCountInString(p.text)
		if used+cost > budget && used > 0 {
			continue
		}
		if used+cost > budget {
			p.text = truncateRunes(p.text, budget-used)
			cost = utf8.RuneCountInString(p.text)
		}
		keep[p.order] = true
		used += cost
	}

	var b strings.Builder
	for _, p := range parts {
		if !keep[p.order] || p.text == "" {
			continue
		}
		b.WriteString("[")
		b.WriteString(p.label)
		b.WriteString("] ")
		b.WriteString(p.text)
		b.WriteByte('\n')
	}
	return SummarySample{
		Text:              strings.TrimRight(b.String(), "\n"),
		IncrementMessages: incMessages,
		IncrementRunes:    incRunes,
		ForkFound:         reachedFork,
	}, nil
}

func sampleMessageText(raw json.RawMessage) (string, bool) {
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &msg) != nil || len(msg.Content) == 0 {
		return "", false
	}
	var plain string
	if json.Unmarshal(msg.Content, &plain) == nil {
		return plain, false
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return "", false
	}
	var texts []string
	hasToolResult := false
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				texts = append(texts, b.Text)
			}
		case "tool_result":
			hasToolResult = true
		}
	}
	return strings.Join(texts, "\n"), hasToolResult
}

func redact(s string) string {
	for _, rule := range redactRules {
		s = rule.re.ReplaceAllString(s, rule.repl)
	}
	return s
}

func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

func totalRunes(list []string) int {
	n := 0
	for _, s := range list {
		n += utf8.RuneCountInString(s)
	}
	return n
}
