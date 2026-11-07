// Author: zhangweiwei
package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"claude-session/internal/model"
)

func TestSessionDisplayProject(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "display.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	pidA, err := st.UpsertProject(ctx, t.TempDir(), "dir-a", "A", 1)
	if err != nil {
		t.Fatal(err)
	}
	pidB, err := st.UpsertProject(ctx, t.TempDir(), "dir-b", "B", 1)
	if err != nil {
		t.Fatal(err)
	}
	id := "3509b3c8-7e44-4373-b56c-6090484dc360"
	sess := model.Session{
		ID: id, ProjectID: pidA, Title: "归属测试",
		FilePath: filepath.Join(t.TempDir(), id+".jsonl"), State: "ok",
	}
	if err := st.UpsertSession(ctx, sess); err != nil {
		t.Fatal(err)
	}

	if err := st.SetSessionProject(ctx, id, &pidB); err != nil {
		t.Fatal(err)
	}
	listA, err := st.ListSessions(ctx, model.SessionFilter{ProjectID: pidA})
	if err != nil {
		t.Fatal(err)
	}
	if len(listA) != 0 {
		t.Errorf("project A sessions = %d, want 0", len(listA))
	}
	listB, err := st.ListSessions(ctx, model.SessionFilter{ProjectID: pidB})
	if err != nil {
		t.Fatal(err)
	}
	if len(listB) != 1 {
		t.Fatalf("project B sessions = %d, want 1", len(listB))
	}
	if listB[0].DisplayProjectID == nil || *listB[0].DisplayProjectID != pidB {
		t.Errorf("display project = %v, want %d", listB[0].DisplayProjectID, pidB)
	}
	if listB[0].FolderID != nil {
		t.Errorf("folder should be cleared on project move")
	}

	folderB, err := st.CreateFolder(ctx, pidB, "B 归类", nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSessionFolder(ctx, id, &folderB.ID); err != nil {
		t.Fatal(err)
	}
	inFolder, err := st.ListSessions(ctx, model.SessionFilter{ProjectID: pidB, FolderID: &folderB.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(inFolder) != 1 {
		t.Errorf("folder sessions = %d, want 1", len(inFolder))
	}

	projects, err := st.ListProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[int64]int{}
	for _, p := range projects {
		counts[p.ID] = p.SessionCount
	}
	if counts[pidA] != 0 || counts[pidB] != 1 {
		t.Errorf("project counts A=%d B=%d, want 0/1", counts[pidA], counts[pidB])
	}

	if err := st.ClearDisplayProject(ctx, pidB); err != nil {
		t.Fatal(err)
	}
	listA, err = st.ListSessions(ctx, model.SessionFilter{ProjectID: pidA})
	if err != nil {
		t.Fatal(err)
	}
	if len(listA) != 1 {
		t.Errorf("after clear, project A sessions = %d, want 1", len(listA))
	}
}

func TestDeleteFolderCascade(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "folders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	pid, err := st.UpsertProject(ctx, t.TempDir(), "encoded", "proj", 1)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := st.CreateFolder(ctx, pid, "父归类", nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	child, err := st.CreateFolder(ctx, pid, "子归类", &parent.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	grand, err := st.CreateFolder(ctx, pid, "孙归类", &child.ID, 1)
	if err != nil {
		t.Fatal(err)
	}

	id := "3509b3c8-7e44-4373-b56c-6090484dc360"
	sess := model.Session{
		ID: id, ProjectID: pid, Title: "多层归类测试", FilePath: filepath.Join(t.TempDir(), id+".jsonl"), State: "ok",
	}
	if err := st.UpsertSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSessionFolder(ctx, id, &grand.ID); err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteFolder(ctx, pid, parent.ID); err != nil {
		t.Fatal(err)
	}
	folders, err := st.ListFolders(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 0 {
		t.Errorf("folders left = %d, want 0 (cascade)", len(folders))
	}
	got, err := st.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.FolderID != nil {
		t.Errorf("session folder = %v, want nil", got.FolderID)
	}
}

func TestFTSSearchAndTrashVisibility(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx := context.Background()
	pid, err := st.UpsertProject(ctx, t.TempDir(), "encoded", "proj", 1)
	if err != nil {
		t.Fatal(err)
	}
	id := "3509b3c8-7e44-4373-b56c-6090484dc360"
	sess := model.Session{
		ID: id, ProjectID: pid, Title: "会话管理系统调研", LastPrompt: "设计一个本地工具",
		MessageCount: 3, FilePath: filepath.Join(t.TempDir(), id+".jsonl"), State: "ok",
	}
	if err := st.UpsertSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	if err := st.IndexSession(ctx, id, sess.Title, sess.LastPrompt, "这里讨论 SQLite 全文检索和会话恢复机制"); err != nil {
		t.Fatal(err)
	}

	got, err := st.ListSessions(ctx, model.SessionFilter{ProjectID: pid, Query: "全文检索"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("fts hits = %d, want 1", len(got))
	}
	if !strings.Contains(got[0].Snippet, "全文检索") {
		t.Errorf("snippet = %q", got[0].Snippet)
	}

	// 少于 3 个字时退回 LIKE
	short, err := st.ListSessions(ctx, model.SessionFilter{ProjectID: pid, Query: "会话"})
	if err != nil {
		t.Fatal(err)
	}
	if len(short) != 1 {
		t.Errorf("short query hits = %d, want 1", len(short))
	}

	if err := st.MarkDeleted(ctx, id, 100); err != nil {
		t.Fatal(err)
	}
	normal, err := st.ListSessions(ctx, model.SessionFilter{ProjectID: pid})
	if err != nil {
		t.Fatal(err)
	}
	if len(normal) != 0 {
		t.Errorf("deleted session leaked into normal list")
	}
	trashed, err := st.ListSessions(ctx, model.SessionFilter{ProjectID: pid, Deleted: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(trashed) != 1 {
		t.Errorf("trash hits = %d, want 1", len(trashed))
	}

	if err := st.MarkRestored(ctx, id); err != nil {
		t.Fatal(err)
	}
	restored, err := st.ListSessions(ctx, model.SessionFilter{ProjectID: pid})
	if err != nil {
		t.Fatal(err)
	}
	if len(restored) != 1 {
		t.Errorf("restored hits = %d, want 1", len(restored))
	}
}

func TestInheritFolderIfEmpty(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "inherit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	pid, err := st.UpsertProject(ctx, t.TempDir(), "encoded", "proj", 1)
	if err != nil {
		t.Fatal(err)
	}
	folder, err := st.CreateFolder(ctx, pid, "归类A", nil, 1)
	if err != nil {
		t.Fatal(err)
	}

	parentID := "11111111-2222-3333-4444-555555555555"
	childID := "22222222-3333-4444-5555-666666666666"
	for _, sess := range []model.Session{
		{ID: parentID, ProjectID: pid, Title: "父会话", FilePath: filepath.Join(t.TempDir(), parentID+".jsonl"), State: "ok"},
		{ID: childID, ProjectID: pid, Title: "子会话", FilePath: filepath.Join(t.TempDir(), childID+".jsonl"), State: "ok"},
	} {
		if err := st.UpsertSession(ctx, sess); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SetSessionFolder(ctx, parentID, &folder.ID); err != nil {
		t.Fatal(err)
	}

	got, found, err := st.SessionFolder(ctx, parentID)
	if err != nil || !found || got == nil || *got != folder.ID {
		t.Fatalf("parent folder = %v, found=%v, err=%v", got, found, err)
	}

	if err := st.InheritFolderIfEmpty(ctx, childID, folder.ID); err != nil {
		t.Fatal(err)
	}
	child, err := st.GetSession(ctx, childID)
	if err != nil {
		t.Fatal(err)
	}
	if child.FolderID == nil || *child.FolderID != folder.ID {
		t.Errorf("child folder = %v, want %d", child.FolderID, folder.ID)
	}

	// 已有归类时不应被继承覆盖
	other, err := st.CreateFolder(ctx, pid, "归类B", nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetSessionFolder(ctx, childID, &other.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.InheritFolderIfEmpty(ctx, childID, folder.ID); err != nil {
		t.Fatal(err)
	}
	child, err = st.GetSession(ctx, childID)
	if err != nil {
		t.Fatal(err)
	}
	if child.FolderID == nil || *child.FolderID != other.ID {
		t.Errorf("child folder overwritten: %v", child.FolderID)
	}
}

func TestAutoDisplayKeepsFolder(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "display-folder.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()

	pidA, err := st.UpsertProject(ctx, t.TempDir(), "dir-a", "A", 1)
	if err != nil {
		t.Fatal(err)
	}
	pidB, err := st.UpsertProject(ctx, t.TempDir(), "dir-b", "B", 1)
	if err != nil {
		t.Fatal(err)
	}
	folder, err := st.CreateFolder(ctx, pidA, "归类A", nil, 1)
	if err != nil {
		t.Fatal(err)
	}

	id := "3509b3c8-7e44-4373-b56c-6090484dc360"
	if err := st.UpsertSession(ctx, model.Session{
		ID: id, ProjectID: pidA, Title: "继续对话测试",
		FilePath: filepath.Join(t.TempDir(), id+".jsonl"), State: "ok",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSessionFolder(ctx, id, &folder.ID); err != nil {
		t.Fatal(err)
	}

	// 模拟继续对话后自动归属漂移到子目录项目
	if err := st.ApplyAutoDisplayProject(ctx, id, &pidB); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.FolderID == nil || *got.FolderID != folder.ID {
		t.Errorf("folder lost after display change: %v", got.FolderID)
	}
	if got.DisplayProjectID == nil || *got.DisplayProjectID != pidB {
		t.Errorf("display project = %v, want %d", got.DisplayProjectID, pidB)
	}

	// 归类视图不受展示项目漂移影响
	list, err := st.ListSessions(ctx, model.SessionFilter{ProjectID: pidA, FolderID: &folder.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Errorf("folder view hits = %d, want 1", len(list))
	}

	// 归属回落到原项目时归类同样保留
	if err := st.ApplyAutoDisplayProject(ctx, id, nil); err != nil {
		t.Fatal(err)
	}
	got, err = st.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.FolderID == nil || *got.FolderID != folder.ID {
		t.Errorf("folder lost after display reset: %v", got.FolderID)
	}
}
