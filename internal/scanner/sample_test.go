// Author: zhangweiwei
package scanner

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSampleForSummaryFiltersNoise(t *testing.T) {
	lines := []string{
		// 工具回填也记为 user，必须剔除。
		`{"type":"user","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","content":"SECRET_TOOL_OUTPUT"}]}}`,
		// 子代理消息。
		`{"type":"user","isSidechain":true,"message":{"role":"user","content":"SIDECHAIN_TEXT"}}`,
		// hook 注入，不是真人输入。
		`{"type":"user","isSidechain":false,"origin":{"kind":"hook"},"message":{"role":"user","content":"HOOK_TEXT"}}`,
		// 无信息量的确认句。
		`{"type":"user","isSidechain":false,"origin":{"kind":"human"},"promptSource":"typed","message":{"role":"user","content":"继续"}}`,
	}
	for i := 1; i <= 12; i++ {
		lines = append(lines, fmt.Sprintf(
			`{"type":"user","isSidechain":false,"origin":{"kind":"human"},"promptSource":"typed","message":{"role":"user","content":"问题 %d：SQLite 并发写入偶发锁冲突"}}`, i))
	}
	lines = append(lines, `{"type":"assistant","isSidechain":false,"message":{"role":"assistant","content":[{"type":"text","text":"结论：改用单协程队列后锁冲突消失。sk-abcdefghijklmnop1234"}]}}`)

	got, err := SampleForSummary(writeLines(t, lines), 1500, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"问题 1", "问题 8", "问题 9", "问题 12", "[结论]"} {
		if !strings.Contains(got, want) {
			t.Errorf("输出缺少 %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"SECRET_TOOL_OUTPUT", "SIDECHAIN_TEXT", "HOOK_TEXT", "继续"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("输出不应包含 %q:\n%s", unwanted, got)
		}
	}
	if strings.Contains(got, "sk-abcdefghijklmnop1234") {
		t.Errorf("密钥未脱敏:\n%s", got)
	}
	if !strings.Contains(got, "[REDACTED_KEY]") {
		t.Errorf("缺少脱敏占位符:\n%s", got)
	}
}

func TestSampleForSummaryBudget(t *testing.T) {
	lines := []string{}
	for i := 1; i <= 20; i++ {
		lines = append(lines, fmt.Sprintf(
			`{"type":"user","isSidechain":false,"origin":{"kind":"human"},"promptSource":"typed","message":{"role":"user","content":"%s"}}`,
			strings.Repeat("长", 80)+fmt.Sprintf("%d", i)))
	}
	path := writeLines(t, lines)
	got, err := SampleForSummary(path, 200, false)
	if err != nil {
		t.Fatal(err)
	}
	// 预算只约束正文，标签会带来少量额外开销。
	if n := utf8.RuneCountInString(got); n > 400 {
		t.Errorf("预算未生效，输出 %d 个字符:\n%s", n, got)
	}
}

func TestSampleForSummaryFromForkPoint(t *testing.T) {
	lines := []string{
		`{"type":"user","uuid":"u1","isSidechain":false,"origin":{"kind":"human"},"promptSource":"typed","message":{"role":"user","content":"父会话的问题"}}`,
		`{"type":"assistant","uuid":"a1","isSidechain":false,"message":{"role":"assistant","content":[{"type":"text","text":"父会话的回答"}]}}`,
		`{"type":"user","uuid":"u2","isSidechain":false,"origin":{"kind":"human"},"promptSource":"typed","message":{"role":"user","content":"分叉后的新问题"}}`,
		`{"type":"assistant","uuid":"a2","isSidechain":false,"message":{"role":"assistant","content":[{"type":"text","text":"分叉后的回答"}]}}`,
	}
	path := writeLines(t, lines)

	got, err := SampleForSummaryFrom(path, "a1", 1500, true)
	if err != nil {
		t.Fatal(err)
	}
	if !got.ForkFound {
		t.Fatal("应定位到分叉点")
	}
	if got.IncrementMessages != 2 {
		t.Fatalf("增量消息数应为 2，实际 %d", got.IncrementMessages)
	}
	if strings.Contains(got.Text, "父会话的问题") || strings.Contains(got.Text, "父会话的回答") {
		t.Errorf("不应包含分叉点之前的内容:\n%s", got.Text)
	}
	if !strings.Contains(got.Text, "分叉后的新问题") {
		t.Errorf("缺少增量内容:\n%s", got.Text)
	}
}

func TestSampleForSummaryFromMissingFork(t *testing.T) {
	lines := []string{
		`{"type":"user","uuid":"u1","isSidechain":false,"origin":{"kind":"human"},"promptSource":"typed","message":{"role":"user","content":"内容"}}`,
	}
	got, err := SampleForSummaryFrom(writeLines(t, lines), "not-exist", 1500, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.ForkFound {
		t.Fatal("分叉点不存在时不应标记 ForkFound")
	}
	if got.Text != "" {
		t.Fatalf("分叉点不存在时应返回空文本，实际 %q", got.Text)
	}
}

func TestSampleForSummaryShortSession(t *testing.T) {
	lines := []string{
		`{"type":"user","isSidechain":false,"origin":{"kind":"human"},"promptSource":"typed","message":{"role":"user","content":"这个报错怎么修"}}`,
		`{"type":"assistant","isSidechain":false,"message":{"role":"assistant","content":[{"type":"text","text":"先看事务边界，再把写操作串行化。"}]}}`,
	}
	got, err := SampleForSummary(writeLines(t, lines), 1500, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "这个报错怎么修") || !strings.Contains(got, "[结论]") {
		t.Errorf("短会话采样不完整:\n%s", got)
	}
}
