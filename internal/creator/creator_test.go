// Author: zhangweiwei
package creator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"claude-session/internal/model"
	"claude-session/internal/scanner"
)

func TestCreateDraft(t *testing.T) {
	projectsDir := t.TempDir()
	projectDir := t.TempDir()
	c := New(projectsDir, "")

	id, err := c.CreateDraft(model.Project{Cwd: projectDir}, "你好，帮我梳理一下模块", "草稿标题")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(projectsDir, scanner.EncodeCwd(projectDir), id+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{`"type":"user"`, `"customTitle":"草稿标题"`, `"sessionId":"` + id + `"`, "你好，帮我梳理一下模块"} {
		if !strings.Contains(content, want) {
			t.Errorf("draft missing %q", want)
		}
	}
}

func TestNewUUID(t *testing.T) {
	id, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 36 || strings.Count(id, "-") != 4 {
		t.Errorf("uuid = %q", id)
	}
}
