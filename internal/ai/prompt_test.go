// Author: zhangweiwei
package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseSummaryStructured(t *testing.T) {
	res := Result{Structured: json.RawMessage(`{"summary":"修复了锁冲突","topics":["SQLite","并发"],"title":"修复 SQLite 锁冲突","confidence":0.9}`)}
	p, err := parseSummary(res)
	if err != nil {
		t.Fatal(err)
	}
	if p.Summary != "修复了锁冲突" || p.Title != "修复 SQLite 锁冲突" || len(p.Topics) != 2 {
		t.Fatalf("解析结果不符: %+v", p)
	}
}

func TestParseSummaryFromTextFallback(t *testing.T) {
	res := Result{Text: `说明文字 {"summary":"从文本解析","topics":["a"],"title":"标题","confidence":0.5} 结尾`}
	p, err := parseSummary(res)
	if err != nil {
		t.Fatal(err)
	}
	if p.Summary != "从文本解析" {
		t.Fatalf("未从文本中提取 JSON: %+v", p)
	}
}

func TestParseSummaryRejectsEmpty(t *testing.T) {
	if _, err := parseSummary(Result{}); err == nil {
		t.Fatal("空结果应当报错")
	}
}

func TestCompactJSON(t *testing.T) {
	raw := json.RawMessage("{\n  \"type\": \"object\",\n  \"properties\": {\"a\": {\"type\": \"string\"}}\n}")
	got := compactJSON(raw)
	if strings.ContainsAny(got, "\n\r") {
		t.Fatalf("压缩后仍含换行: %q", got)
	}
	if !json.Valid([]byte(got)) {
		t.Fatalf("压缩结果不是合法 JSON: %q", got)
	}
}

func TestParseCLIOutputSkipsWarningLine(t *testing.T) {
	stdout := "[claude-code:unrecognized_model] {\"model\":\"x\"}\n" +
		`{"type":"result","result":"ok","structured_output":{"summary":"s"},"usage":{"input_tokens":7,"output_tokens":3},"total_cost_usd":0.01}`
	out, err := parseCLIOutput(stdout)
	if err != nil {
		t.Fatal(err)
	}
	if out.Result != "ok" || out.Usage.InputTokens != 7 || out.TotalCostUSD != 0.01 {
		t.Fatalf("解析结果不符: %+v", out)
	}
}

func TestClassifyCLIError(t *testing.T) {
	cases := []struct {
		msg  string
		kind Kind
	}{
		{"Error: rate limit exceeded", KindRateLimit},
		{"not logged in, please run login", KindAuth},
		{"no such file or directory", KindUnavailable},
		{"something else", KindModel},
	}
	for _, c := range cases {
		err := classifyCLIError(c.msg)
		if kindOf(err) != c.kind {
			t.Errorf("classifyCLIError(%q) = %v, want %v", c.msg, kindOf(err), c.kind)
		}
	}
}
