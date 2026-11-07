// Author: zhangweiwei
package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"claude-session/internal/store"
)

func writeLines(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseMeta(t *testing.T) {
	path := writeLines(t, []string{
		`{"type":"user","uuid":"u1","parentUuid":null,"isSidechain":false,"timestamp":"2026-11-02T10:00:00.000Z","cwd":"C:\\work\\proj","gitBranch":"main","version":"2.1.316","message":{"role":"user","content":"你好"}}`,
		`{"type":"user","uuid":"u2","parentUuid":"u1","isSidechain":true,"timestamp":"2026-11-02T10:00:01.000Z","cwd":"C:\\work\\proj","message":{"role":"user","content":"侧链消息"}}`,
		`{"type":"assistant","uuid":"u3","parentUuid":"u2","isSidechain":false,"timestamp":"2026-11-02T10:00:02.000Z","cwd":"C:\\work\\proj","message":{"role":"assistant","content":[{"type":"text","text":"你好"},{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}`,
		`{"type":"ai-title","aiTitle":"问候"}`,
		`{"type":"custom-title","customTitle":"自定义标题"}`,
		`{"type":"last-prompt","lastPrompt":"最后一句"}`,
	})

	m, err := parseMeta(path, "session-id")
	if err != nil {
		t.Fatal(err)
	}
	if m.Cwd != `C:\work\proj` {
		t.Errorf("Cwd = %q", m.Cwd)
	}
	if m.MessageCount != 2 {
		t.Errorf("MessageCount = %d, want 2", m.MessageCount)
	}
	if m.AITitle != "问候" || m.CustomTitle != "自定义标题" {
		t.Errorf("titles = %q / %q", m.AITitle, m.CustomTitle)
	}
	if m.LastPrompt != "最后一句" {
		t.Errorf("LastPrompt = %q", m.LastPrompt)
	}
	if m.FirstTS != "2026-11-02T10:00:00.000Z" || m.LastTS != "2026-11-02T10:00:02.000Z" {
		t.Errorf("timestamps = %q / %q", m.FirstTS, m.LastTS)
	}
	if got := buildTitle(m.CustomTitle, m.AITitle, m.LastPrompt, "id"); got != "自定义标题" {
		t.Errorf("title = %q", got)
	}
}

func TestParseMetaForkedFrom(t *testing.T) {
	path := writeLines(t, []string{
		`{"type":"user","uuid":"u1","sessionId":"child-id","forkedFrom":{"sessionId":"11111111-2222-3333-4444-555555555555","messageUuid":"point-1"},"isSidechain":false,"timestamp":"2026-11-02T10:00:00.000Z","cwd":"C:\\work\\proj","message":{"role":"user","content":"从父会话复制"}}`,
	})

	m, err := parseMeta(path, "child-id")
	if err != nil {
		t.Fatal(err)
	}
	if m.ParentID != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("ParentID = %q", m.ParentID)
	}
	if m.ForkPoint != "point-1" {
		t.Errorf("ForkPoint = %q", m.ForkPoint)
	}
}

func TestParseMetaIgnoresInvalidForkedFrom(t *testing.T) {
	path := writeLines(t, []string{
		`{"type":"user","uuid":"u1","forkedFrom":{"sessionId":"not-a-uuid","messageUuid":"point-1"},"isSidechain":false,"timestamp":"2026-11-02T10:00:00.000Z","cwd":"C:\\work\\proj","message":{"role":"user","content":"脏数据"}}`,
	})

	m, err := parseMeta(path, "child-id")
	if err != nil {
		t.Fatal(err)
	}
	if m.ParentID != "" || m.ForkPoint != "" {
		t.Errorf("非法 forkedFrom 应被忽略，got %q / %q", m.ParentID, m.ForkPoint)
	}
}

func TestReadMessages(t *testing.T) {
	path := writeLines(t, []string{
		`{"type":"user","uuid":"u1","isSidechain":false,"timestamp":"2026-11-02T10:00:00.000Z","message":{"role":"user","content":"第一句"}}`,
		`{"type":"user","uuid":"u2","isSidechain":true,"timestamp":"2026-11-02T10:00:01.000Z","message":{"role":"user","content":"侧链"}}`,
		`{"type":"assistant","uuid":"u3","isSidechain":false,"timestamp":"2026-11-02T10:00:02.000Z","message":{"role":"assistant","content":[{"type":"text","text":"回答"},{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}`,
		`{"type":"user","uuid":"u4","isSidechain":false,"timestamp":"2026-11-02T10:00:03.000Z","message":{"role":"user","content":[{"type":"tool_result","content":"ok"}]}}`,
		`{"type":"attachment","sessionId":"x"}`,
	})

	msgs, truncated, err := ReadMessages(path)
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Error("unexpected truncated")
	}
	if len(msgs) != 2 {
		t.Fatalf("len = %d, want 2", len(msgs))
	}
	if msgs[0].Type != "user" || msgs[0].Text != "第一句" {
		t.Errorf("first = %+v", msgs[0])
	}
	if msgs[1].Type != "assistant" || msgs[1].Text != "回答" || len(msgs[1].Tools) != 1 {
		t.Errorf("second = %+v", msgs[1])
	}
	if msgs[1].Tools[0].Name != "Bash" {
		t.Errorf("tool = %+v", msgs[1].Tools[0])
	}
}

func TestIsSessionID(t *testing.T) {
	ok := "3509b3c8-7e44-4373-b56c-6090484dc360"
	if !isSessionID(ok) {
		t.Error("valid uuid rejected")
	}
	for _, bad := range []string{"agent-123", "3509b3c8", "3509b3c8-7e44-4373-b56c-6090484dc36z"} {
		if isSessionID(bad) {
			t.Errorf("invalid id accepted: %q", bad)
		}
	}
}

func TestScanInheritsParentFolder(t *testing.T) {
	root := t.TempDir()
	projectsDir := filepath.Join(root, "projects")
	dir := filepath.Join(projectsDir, "-tmp-demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(root, "sessions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	parentID := "11111111-2222-3333-4444-555555555555"
	childID := "22222222-3333-4444-5555-666666666666"
	writeSessionFile(t, dir, parentID,
		`{"type":"user","uuid":"u1","isSidechain":false,"timestamp":"2026-11-05T10:00:00.000Z","cwd":"D:\\tmp\\demo","gitBranch":"main","version":"2.1.316","message":{"role":"user","content":"父会话"}}`)

	sc := New(projectsDir, st)
	if _, err := sc.Scan(ctx); err != nil {
		t.Fatal(err)
	}

	projects, err := st.ListProjects(ctx)
	if err != nil || len(projects) == 0 {
		t.Fatalf("projects = %v, err = %v", projects, err)
	}
	folder, err := st.CreateFolder(ctx, projects[0].ID, "归类A", nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSessionFolder(ctx, parentID, &folder.ID); err != nil {
		t.Fatal(err)
	}

	writeSessionFile(t, dir, childID, fmt.Sprintf(
		`{"type":"user","uuid":"u2","isSidechain":false,"timestamp":"2026-11-05T11:00:00.000Z","cwd":"D:\\tmp\\demo","gitBranch":"main","version":"2.1.316","forkedFrom":{"sessionId":"%s","messageUuid":"u1"},"message":{"role":"user","content":"子会话"}}`,
		parentID))

	if _, err := sc.Scan(ctx); err != nil {
		t.Fatal(err)
	}

	child, err := st.GetSession(ctx, childID)
	if err != nil {
		t.Fatal(err)
	}
	if child.FolderID == nil || *child.FolderID != folder.ID {
		t.Errorf("child folder = %v, want %d", child.FolderID, folder.ID)
	}
}

func writeSessionFile(t *testing.T, dir, id, line string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
