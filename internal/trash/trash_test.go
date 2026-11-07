// Author: zhangweiwei
package trash

import (
	"os"
	"path/filepath"
	"testing"

	"claude-session/internal/model"
)

func TestMoveRestorePurge(t *testing.T) {
	root := t.TempDir()
	srcDir := t.TempDir()
	id := "3509b3c8-7e44-4373-b56c-6090484dc360"
	file := filepath.Join(srcDir, id+".jsonl")
	if err := os.WriteFile(file, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	attachments := filepath.Join(srcDir, id)
	if err := os.MkdirAll(attachments, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attachments, "result.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(root)
	sess := model.Session{ID: id, FilePath: file}

	if err := m.Move(sess); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("session file still exists after move")
	}
	if _, err := os.Stat(filepath.Join(root, id, "session.jsonl")); err != nil {
		t.Errorf("trash file missing: %v", err)
	}

	if err := m.Restore(sess); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Errorf("session file not restored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(attachments, "result.txt")); err != nil {
		t.Errorf("attachments not restored: %v", err)
	}

	if err := m.Move(sess); err != nil {
		t.Fatal(err)
	}
	if err := m.Purge(sess); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, id)); !os.IsNotExist(err) {
		t.Error("trash directory still exists after purge")
	}
}
