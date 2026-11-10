// Author: zhangweiwei
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Profile 是 CLI 调用的降级档位：bare 固定开销最低，plain 最兼容。
type Profile string

const (
	ProfileBare  Profile = "bare"
	ProfileLean  Profile = "lean"
	ProfilePlain Profile = "plain"
)

type Request struct {
	System string
	User   string
	Schema json.RawMessage
}

type Result struct {
	Text       string
	Structured json.RawMessage
	Model      string
	Profile    Profile
	InTokens   int
	OutTokens  int
	CostUSD    float64
	Duration   time.Duration
}

type Generator interface {
	Name() string
	Generate(ctx context.Context, req Request) (Result, error)
	Probe(ctx context.Context) (Profile, string, error)
}

// Kind 是可供调用方分类处理的错误类型。
type Kind int

const (
	KindUnavailable Kind = iota
	KindAuth
	KindRateLimit
	KindTimeout
	KindModel
	KindSchema
)

type Error struct {
	Kind Kind
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

func kindOf(err error) Kind {
	var ae *Error
	if errors.As(err, &ae) {
		return ae.Kind
	}
	return KindModel
}

// CLIGenerator 通过本机 Claude Code 的 print 模式生成内容。
type CLIGenerator struct {
	mu        sync.RWMutex
	bin       string
	model     string
	workDir   string
	profile   Profile
	lastError string
}

func NewCLIGenerator(bin, model, workDir string) *CLIGenerator {
	return &CLIGenerator{bin: bin, model: model, workDir: workDir, profile: ProfilePlain}
}

func (g *CLIGenerator) Name() string { return "claude-cli" }

func (g *CLIGenerator) Available() bool { return g != nil && g.bin != "" }

func (g *CLIGenerator) SetModel(name string) {
	g.mu.Lock()
	g.model = name
	g.mu.Unlock()
}

// Probe 依次尝试 bare / lean / plain，返回第一个可用的档位与 CLI 版本。
func (g *CLIGenerator) Probe(ctx context.Context) (Profile, string, error) {
	if !g.Available() {
		return "", "", &Error{Kind: KindUnavailable, Msg: "未找到 claude 可执行文件"}
	}
	version := g.version(ctx)
	for _, p := range []Profile{ProfileBare, ProfileLean, ProfilePlain} {
		g.mu.Lock()
		g.profile = p
		g.mu.Unlock()
		probeCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		_, err := g.Generate(probeCtx, Request{
			System: "只输出 JSON",
			User:   `{"ok":true}`,
			Schema: json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`),
		})
		cancel()
		if err == nil {
			return p, version, nil
		}
		g.mu.Lock()
		g.lastError = err.Error()
		g.mu.Unlock()
		if kindOf(err) == KindAuth && p != ProfilePlain {
			continue
		}
	}
	return "", version, &Error{Kind: KindUnavailable, Msg: "AI 调用不可用: " + g.lastError}
}

func (g *CLIGenerator) version(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, g.bin, "--version")
	if ext := strings.ToLower(filepath.Ext(g.bin)); ext == ".cmd" || ext == ".bat" {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/c", g.bin, "--version")
	}
	hideConsole(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (g *CLIGenerator) Generate(ctx context.Context, req Request) (Result, error) {
	if !g.Available() {
		return Result{}, &Error{Kind: KindUnavailable, Msg: "未找到 claude 可执行文件"}
	}
	g.mu.RLock()
	profile := g.profile
	model := g.model
	g.mu.RUnlock()
	if profile == "" {
		profile = ProfilePlain
	}

	args := []string{
		"-p",
		"--output-format", "json",
		"--no-session-persistence",
		"--tools", "",
	}
	if len(req.Schema) > 0 {
		// Windows 命令行不接受换行，schema 必须压成单行再传。
		args = append(args, "--json-schema", compactJSON(req.Schema))
	}
	switch profile {
	case ProfileBare:
		args = append(args, "--bare")
	case ProfileLean:
		args = append(args, "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`)
	}
	if req.System != "" {
		args = append(args, "--append-system-prompt", req.System)
	}
	if model != "" {
		args = append(args, "--model", model)
	}

	var cmd *exec.Cmd
	ext := strings.ToLower(filepath.Ext(g.bin))
	if ext == ".cmd" || ext == ".bat" {
		cmd = exec.CommandContext(ctx, "cmd.exe", append([]string{"/c", g.bin}, args...)...)
	} else {
		cmd = exec.CommandContext(ctx, g.bin, args...)
	}
	hideConsole(cmd)
	if g.workDir != "" {
		_ = os.MkdirAll(g.workDir, 0o755)
		cmd.Dir = g.workDir
	}
	cmd.Stdin = strings.NewReader(req.User)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)
	if err != nil {
		if ctx.Err() != nil {
			return Result{}, &Error{Kind: KindTimeout, Msg: "模型调用超时"}
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return Result{}, classifyCLIError(msg)
	}

	out, parseErr := parseCLIOutput(stdout.String())
	if parseErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			parseErr = fmt.Errorf("%v: %s", parseErr, truncate(msg, 300))
		}
		return Result{}, parseErr
	}

	res := Result{
		Text:       out.Result,
		Structured: out.StructuredOutput,
		Profile:    profile,
		InTokens:   out.Usage.InputTokens,
		OutTokens:  out.Usage.OutputTokens,
		CostUSD:    out.TotalCostUSD,
		Duration:   elapsed,
	}
	for name := range out.ModelUsage {
		res.Model = name
		break
	}
	if out.IsError {
		return res, &Error{Kind: KindModel, Msg: "模型返回错误: " + truncate(out.Result, 300)}
	}
	if len(req.Schema) > 0 && len(res.Structured) == 0 {
		return res, &Error{Kind: KindSchema, Msg: "模型未返回结构化结果"}
	}
	return res, nil
}

type cliOutput struct {
	Type             string          `json:"type"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	IsError          bool            `json:"is_error"`
	APITErrorStatus  *int            `json:"api_error_status"`
	TotalCostUSD     float64         `json:"total_cost_usd"`
	Usage            struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	ModelUsage map[string]json.RawMessage `json:"modelUsage"`
}

// parseCLIOutput 跳过混在 stdout 里的警告行，取 type=result 的那一条。
func parseCLIOutput(stdout string) (cliOutput, error) {
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var out cliOutput
		if json.Unmarshal([]byte(line), &out) != nil {
			continue
		}
		if out.Type == "result" {
			if out.APITErrorStatus != nil && *out.APITErrorStatus == 429 {
				return out, &Error{Kind: KindRateLimit, Msg: "模型限流，请稍后重试"}
			}
			return out, nil
		}
	}
	return cliOutput{}, &Error{Kind: KindModel, Msg: "无法解析模型输出"}
}

func classifyCLIError(msg string) error {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "rate") || strings.Contains(lower, "429") || strings.Contains(lower, "quota"):
		return &Error{Kind: KindRateLimit, Msg: "模型限流: " + truncate(msg, 300)}
	case strings.Contains(lower, "auth") || strings.Contains(lower, "login") || strings.Contains(lower, "keychain") ||
		strings.Contains(lower, "api key") || strings.Contains(lower, "unauthorized"):
		return &Error{Kind: KindAuth, Msg: "Claude Code 未登录或认证失败: " + truncate(msg, 300)}
	case strings.Contains(lower, "no such file") || strings.Contains(lower, "not found"):
		return &Error{Kind: KindUnavailable, Msg: "未找到 claude 可执行文件: " + truncate(msg, 300)}
	default:
		return &Error{Kind: KindModel, Msg: "模型调用失败: " + truncate(msg, 300)}
	}
}

func compactJSON(raw json.RawMessage) string {
	var buf bytes.Buffer
	if json.Compact(&buf, raw) != nil {
		return string(raw)
	}
	return buf.String()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
