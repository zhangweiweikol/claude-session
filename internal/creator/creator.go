// Author: zhangweiwei
package creator

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"claude-session/internal/model"
	"claude-session/internal/scanner"
)

type Creator struct {
	projectsDir string
	claudePath  string
}

func New(projectsDir, claudePath string) *Creator {
	return &Creator{projectsDir: projectsDir, claudePath: claudePath}
}

func (c *Creator) Available() bool { return c.claudePath != "" }

// Fork 把源会话的全部历史复制成新会话文件，源文件保持不变。
// 新文件沿用源文件所在目录，逐行替换 sessionId，并在首行写入 forkedFrom 元数据，
// 与 Claude Code 的 /branch、/fork 磁盘结构保持一致。
func (c *Creator) Fork(sourcePath, sourceID, title string) (string, error) {
	id, err := newUUID()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(sourcePath)
	dst := filepath.Join(dir, id+".jsonl")

	src, err := os.Open(sourcePath)
	if err != nil {
		return "", fmt.Errorf("读取会话文件失败: %w", err)
	}
	defer src.Close()

	var out bytes.Buffer
	sc := bufio.NewScanner(src)
	sc.Buffer(make([]byte, 256<<10), 32<<20)
	first := true
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var row map[string]any
		if json.Unmarshal(line, &row) != nil {
			continue
		}
		row["sessionId"] = id
		if first {
			first = false
			startUUID, _ := row["uuid"].(string)
			row["forkedFrom"] = map[string]any{
				"sessionId":   sourceID,
				"messageUuid": startUUID,
			}
		}
		data, err := json.Marshal(row)
		if err != nil {
			return "", err
		}
		out.Write(data)
		out.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("解析会话文件失败: %w", err)
	}
	if out.Len() == 0 {
		return "", errors.New("会话文件为空，无法创建分支")
	}
	if err := os.WriteFile(dst, out.Bytes(), 0o644); err != nil {
		return "", fmt.Errorf("写入分支会话失败: %w", err)
	}
	if title != "" {
		if err := scanner.AppendCustomTitle(dst, id, title); err != nil {
			return "", fmt.Errorf("写入分支标题失败: %w", err)
		}
	}
	return id, nil
}

func (c *Creator) CreateDraft(project model.Project, firstMessage, title string) (string, error) {
	id, err := newUUID()
	if err != nil {
		return "", err
	}
	messageID, err := newUUID()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(c.projectsDir, scanner.EncodeCwd(project.Cwd))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建会话目录失败: %w", err)
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	rows := []map[string]any{
		{
			"parentUuid": nil, "isSidechain": false, "promptId": messageID,
			"type": "user", "message": map[string]any{"role": "user", "content": firstMessage},
			"uuid": messageID, "timestamp": now,
			"origin": map[string]any{"kind": "human"}, "promptSource": "typed", "turnOrigin": "human",
			"turnPosition": map[string]any{"promptIndex": 1, "turnIndex": 1},
			"userType":     "external", "entrypoint": "cli",
			"cwd": project.Cwd, "sessionId": id,
		},
		{"type": "last-prompt", "lastPrompt": firstMessage, "leafUuid": messageID, "sessionId": id},
	}
	if title != "" {
		rows = append(rows, map[string]any{"type": "custom-title", "customTitle": title, "sessionId": id})
	}

	var content strings.Builder
	for _, row := range rows {
		data, err := json.Marshal(row)
		if err != nil {
			return "", err
		}
		content.Write(data)
		content.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(content.String()), 0o644); err != nil {
		return "", fmt.Errorf("写入会话文件失败: %w", err)
	}
	return id, nil
}

// Compact 调用 Claude Code 的 /compact 命令压缩指定会话的上下文。
func (c *Creator) Compact(ctx context.Context, cwd, sessionID string) error {
	if c.claudePath == "" {
		return errors.New("未找到 claude 可执行文件")
	}
	args := []string{"--resume", sessionID, "-p", "/compact", "--output-format", "json"}

	var cmd *exec.Cmd
	ext := strings.ToLower(filepath.Ext(c.claudePath))
	if ext == ".cmd" || ext == ".bat" {
		cmd = exec.CommandContext(ctx, "cmd.exe", append([]string{"/c", c.claudePath}, args...)...)
	} else {
		cmd = exec.CommandContext(ctx, c.claudePath, args...)
	}
	cmd.Dir = cwd

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return errors.New("压缩超时，请稍后重试")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("Claude Code 压缩失败: %s", truncate(msg, 400))
	}
	return nil
}

func (c *Creator) CreateCLI(ctx context.Context, project model.Project, firstMessage, title string) (string, error) {
	if c.claudePath == "" {
		return "", errors.New("未找到 claude 可执行文件")
	}
	id, err := newUUID()
	if err != nil {
		return "", err
	}

	args := []string{"--session-id", id, "-p", "--output-format", "json"}
	if title != "" {
		args = append(args, "--name", title)
	}

	var cmd *exec.Cmd
	ext := strings.ToLower(filepath.Ext(c.claudePath))
	if ext == ".cmd" || ext == ".bat" {
		cmd = exec.CommandContext(ctx, "cmd.exe", append([]string{"/c", c.claudePath}, args...)...)
	} else {
		cmd = exec.CommandContext(ctx, c.claudePath, args...)
	}
	cmd.Dir = project.Cwd
	cmd.Stdin = strings.NewReader(firstMessage)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("Claude Code 创建会话失败: %s", truncate(msg, 400))
	}

	if actual := parseSessionID(stdout.String()); actual != "" {
		id = actual
	}

	// 给 Claude Code 一点时间把文件落盘；即便超时也返回 ID，交给后续扫描。
	path := filepath.Join(c.projectsDir, scanner.EncodeCwd(project.Cwd), id+".jsonl")
	for i := 0; i < 30; i++ {
		if _, err := os.Stat(path); err == nil {
			return id, nil
		}
		if ctx.Err() != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return id, nil
}

func parseSessionID(output string) string {
	var v struct {
		SessionID string `json:"session_id"`
	}
	text := strings.TrimSpace(output)
	if json.Unmarshal([]byte(text), &v) == nil && v.SessionID != "" {
		return v.SessionID
	}
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if json.Unmarshal([]byte(strings.TrimSpace(lines[i])), &v) == nil && v.SessionID != "" {
			return v.SessionID
		}
	}
	return ""
}

func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
