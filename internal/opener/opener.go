// Author: zhangweiwei
package opener

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf16"
)

var (
	ErrInvalidID  = errors.New("会话 ID 格式不合法")
	ErrInvalidCwd = errors.New("项目路径不合法或不存在")
	ErrNoClaude   = errors.New("未找到 claude 可执行文件，请确认 Claude Code 已安装并在 PATH 中")
	ErrBusy       = errors.New("该会话正在 Claude Code 中运行")
	ErrNoTerminal = errors.New("无法启动终端，请复制恢复命令手动执行")
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// sessionEnvPrelude 清除从当前 Claude Code 会话继承的子会话标记。
// 否则新打开的会话会关闭 transcript 保存，并继承当前会话的 ID 与消息管道。
const sessionEnvPrelude = `$env:CLAUDE_CODE_CHILD_SESSION=$null;$env:CLAUDE_CODE_SESSION_ID=$null;$env:CLAUDE_CODE_SESSION_ATTENDED=$null;$env:CLAUDE_CODE_MESSAGING_SOCKET=$null;$env:CLAUDE_CODE_MESSAGING_TOKEN=$null;$env:CLAUDE_CODE_SUBAGENT_MODEL=$null;$env:CLAUDE_CODE_FORCE_SESSION_PERSISTENCE='1';`

var sessionEnvKeys = []string{
	"CLAUDE_CODE_CHILD_SESSION",
	"CLAUDE_CODE_SESSION_ID",
	"CLAUDE_CODE_SESSION_ATTENDED",
	"CLAUDE_CODE_MESSAGING_SOCKET",
	"CLAUDE_CODE_MESSAGING_TOKEN",
	"CLAUDE_CODE_SUBAGENT_MODEL",
}

func sanitizedEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		if isSessionEnvKey(key) {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1")
}

func isSessionEnvKey(key string) bool {
	for _, k := range sessionEnvKeys {
		if strings.EqualFold(key, k) {
			return true
		}
	}
	return false
}

type Result struct {
	Terminal string `json:"terminal"`
	Shell    string `json:"shell"`
	Command  string `json:"command"`
}

type Opener struct {
	wt          string
	pwsh        string
	powershell  string
	claude      string
	sessionsDir string
}

func New(sessionsDir string) *Opener {
	o := &Opener{sessionsDir: sessionsDir}
	if runtime.GOOS == "windows" {
		o.wt = lookPath("wt.exe")
		o.pwsh = lookPath("pwsh.exe")
		o.powershell = lookPath("powershell.exe")
		o.claude = lookPath("claude.cmd")
		if o.claude == "" {
			o.claude = lookPath("claude")
		}
	}
	return o
}

func (o *Opener) Available() bool {
	return o.wt != "" || o.pwsh != "" || o.powershell != ""
}

func (o *Opener) SessionsDir() string { return o.sessionsDir }

func (o *Opener) ClaudePath() string { return o.claude }

// SessionBusy 供标题写入等需要并发保护的场景复用。
func SessionBusy(sessionsDir, sessionID string) bool {
	return isSessionBusy(sessionsDir, sessionID)
}

func (o *Opener) Open(cwd, sessionID string) (Result, error) {
	if !uuidRe.MatchString(sessionID) {
		return Result{}, ErrInvalidID
	}
	if !validCwd(cwd) {
		return Result{}, ErrInvalidCwd
	}
	if o.claude == "" {
		return Result{}, ErrNoClaude
	}
	if isSessionBusy(o.sessionsDir, sessionID) {
		return Result{}, ErrBusy
	}

	terminal, shellName, args, err := o.plan(cwd, sessionID)
	if err != nil {
		return Result{}, err
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = sanitizedEnv()
	if err := cmd.Start(); err != nil {
		if terminal == "windows-terminal" {
			if fbErr := o.fallback(cwd, sessionID); fbErr == nil {
				return Result{
					Terminal: "cmd-start",
					Shell:    shellName,
					Command:  resumeCommand(sessionID),
				}, nil
			}
		}
		return Result{}, fmt.Errorf("%w: %v", ErrNoTerminal, err)
	}
	return Result{
		Terminal: terminal,
		Shell:    shellName,
		Command:  resumeCommand(sessionID),
	}, nil
}

// plan 返回最终要执行的命令。把参数拆开传递，避免 wt 重新拼接命令行时的引号问题。
func (o *Opener) plan(cwd, sessionID string) (terminal, shellName string, args []string, err error) {
	shell := o.pwsh
	shellName = "pwsh"
	if shell == "" {
		shell = o.powershell
		shellName = "powershell"
	}
	if shell == "" {
		return "", "", nil, ErrNoTerminal
	}
	encoded := encodePowerShellCommand(resumeScript(sessionID))
	if o.wt == "" {
		return "cmd-start", shellName, []string{
			"cmd.exe", "/c", "start", "", shell, "-NoExit", "-EncodedCommand", encoded,
		}, nil
	}
	// -w 0（last）：在最近使用的 Windows Terminal 窗口中新建标签页并前置；
	// 没有已打开的窗口时由 wt 自动新建，等价于原来的行为。
	return "windows-terminal", shellName, []string{
		o.wt, "-w", "0", "nt", "-d", cwd, shell, "-NoExit", "-EncodedCommand", encoded,
	}, nil
}

func resumeScript(sessionID string) string {
	return sessionEnvPrelude + " claude --resume " + sessionID
}

// encodePowerShellCommand 把脚本转成 -EncodedCommand 需要的 UTF-16LE base64。
// base64 不含空格和引号，经过 wt / cmd 转发时不会被重新拆分。
func encodePowerShellCommand(script string) string {
	units := utf16.Encode([]rune(script))
	buf := make([]byte, len(units)*2)
	for i, u := range units {
		buf[i*2] = byte(u)
		buf[i*2+1] = byte(u >> 8)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func (o *Opener) fallback(cwd, sessionID string) error {
	shell := o.pwsh
	if shell == "" {
		shell = o.powershell
	}
	if shell == "" {
		return ErrNoTerminal
	}
	cmd := exec.Command("cmd.exe", "/c", "start", "", shell, "-NoExit",
		"-EncodedCommand", encodePowerShellCommand(resumeScript(sessionID)))
	cmd.Dir = cwd
	cmd.Env = sanitizedEnv()
	return cmd.Start()
}

func resumeCommand(sessionID string) string {
	return "claude --resume " + sessionID
}

func validCwd(cwd string) bool {
	if cwd == "" || !filepath.IsAbs(cwd) {
		return false
	}
	if strings.ContainsAny(cwd, "\r\n\"\x00") {
		return false
	}
	info, err := os.Stat(cwd)
	return err == nil && info.IsDir()
}

// ActiveSessions 返回当前正在 Claude Code 中运行的会话 ID 集合。
func ActiveSessions(sessionsDir string) map[string]bool {
	out := make(map[string]bool)
	files, err := filepath.Glob(filepath.Join(sessionsDir, "*.json"))
	if err != nil {
		return out
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var v struct {
			SessionID string `json:"sessionId"`
			Status    string `json:"status"`
		}
		if json.Unmarshal(data, &v) != nil {
			continue
		}
		if v.SessionID != "" && v.Status != "" && v.Status != "idle" {
			out[v.SessionID] = true
		}
	}
	return out
}

func isSessionBusy(sessionsDir, sessionID string) bool {
	return ActiveSessions(sessionsDir)[sessionID]
}

func lookPath(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}
