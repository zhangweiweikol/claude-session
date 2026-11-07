// Author: zhangweiwei
package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"claude-session/internal/model"
)

func newAITestStore(t *testing.T) (*Store, context.Context, int64) {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "ai.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	pid, err := st.UpsertProject(ctx, t.TempDir(), "dir", "proj", time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	return st, ctx, pid
}

func TestAISummaryRoundTrip(t *testing.T) {
	st, ctx, pid := newAITestStore(t)
	mtime := time.Now().Add(-2 * time.Minute).Unix()
	sess := model.Session{
		ID: "11111111-1111-4111-8111-111111111111", ProjectID: pid,
		Title: "锁冲突", FilePath: filepath.Join(t.TempDir(), "a.jsonl"),
		FileSize: 120, FileMtime: mtime, MessageCount: 4, State: "ok",
	}
	if err := st.UpsertSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertAISummary(ctx, model.AISummary{
		SessionID: sess.ID, Summary: "修复了并发锁冲突", SuggestedTitle: "修复 SQLite 锁冲突",
		Topics: []string{"SQLite", "并发"}, Status: "ok", Confidence: 0.86,
		SourceMtime: mtime, SourceSize: 120, CostUSD: 0.01, InTokens: 900,
	}); err != nil {
		t.Fatal(err)
	}

	got, found, err := st.GetAISummary(ctx, sess.ID)
	if err != nil || !found {
		t.Fatalf("读取概要失败: found=%v err=%v", found, err)
	}
	if got.Summary != "修复了并发锁冲突" || got.SuggestedTitle != "修复 SQLite 锁冲突" || len(got.Topics) != 2 {
		t.Fatalf("概要内容不符: %+v", got)
	}

	list, err := st.ListSessions(ctx, model.SessionFilter{ProjectID: pid, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].AISummary == "" || !list[0].AICanApplyTitle {
		t.Fatalf("列表未附加 AI 字段: %+v", list)
	}

	count, failed, in, out, cost, err := st.AITotals(ctx)
	if err != nil || count != 1 || failed != 0 || in != 900 || out != 0 || cost == 0 {
		t.Fatalf("用量汇总不符: count=%d failed=%d in=%d out=%d cost=%f err=%v", count, failed, in, out, cost, err)
	}
}

func TestCategorizeCandidatesUsesDisplayProject(t *testing.T) {
	st, ctx, pidA := newAITestStore(t)
	pidB, err := st.UpsertProject(ctx, t.TempDir(), "dir-b", "B", time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	id := "66666666-6666-4666-8666-666666666666"
	if err := st.UpsertSession(ctx, model.Session{
		ID: id, ProjectID: pidA, Title: "漂移会话", FilePath: filepath.Join(t.TempDir(), "d.jsonl"),
		FileSize: 10, FileMtime: time.Now().Add(-2 * time.Minute).Unix(), MessageCount: 3, State: "ok",
	}); err != nil {
		t.Fatal(err)
	}
	// 会话创建在 A，继续对话时漂移到 B。
	if err := st.SetSessionProject(ctx, id, &pidB); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertAISummary(ctx, model.AISummary{SessionID: id, Summary: "d", Status: "ok"}); err != nil {
		t.Fatal(err)
	}

	got, err := st.CategorizeCandidates(ctx, pidB, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != id {
		t.Fatalf("展示项目下应能归类，实际 %+v", got)
	}
}

func TestResetActiveJobs(t *testing.T) {
	st, ctx, pid := newAITestStore(t)
	jobID := "job-running"
	if err := st.CreateAIJob(ctx, model.AIJob{
		ID: jobID, Kind: "categorize", ProjectID: pid, Status: "running", Total: 3,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.ResetActiveJobs(ctx); err != nil {
		t.Fatal(err)
	}
	job, err := st.GetAIJob(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "failed" || job.Error == "" {
		t.Fatalf("残留任务应被标记为失败: %+v", job)
	}
}

func TestFTSSearchMatchesSummary(t *testing.T) {
	st, ctx, pid := newAITestStore(t)
	id := "55555555-5555-4555-8555-555555555555"
	mtime := time.Now().Add(-2 * time.Minute).Unix()
	if err := st.UpsertSession(ctx, model.Session{
		ID: id, ProjectID: pid, Title: "并发改造", FilePath: filepath.Join(t.TempDir(), "f.jsonl"),
		FileSize: 10, FileMtime: mtime, MessageCount: 3, State: "ok",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.IndexSession(ctx, id, "并发改造", "改一下扫描", "正文只讨论队列调度"); err != nil {
		t.Fatal(err)
	}

	// 概要生成前，摘要词不应命中。
	before, err := st.ListSessions(ctx, model.SessionFilter{ProjectID: pid, Query: "数据库锁", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 0 {
		t.Fatalf("摘要未回填时不应命中，实际 %d", len(before))
	}

	if err := st.UpsertAISummary(ctx, model.AISummary{
		SessionID: id, Summary: "排查数据库锁与并发写入冲突", Status: "ok",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateFTSSummary(ctx, id); err != nil {
		t.Fatal(err)
	}
	after, err := st.ListSessions(ctx, model.SessionFilter{ProjectID: pid, Query: "数据库锁", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].ID != id {
		t.Fatalf("应通过摘要命中会话，实际 %+v", after)
	}
}

func TestPendingSummarySessions(t *testing.T) {
	st, ctx, pid := newAITestStore(t)
	mtime := time.Now().Add(-2 * time.Minute).Unix()
	mk := func(id, title string, size int64) model.Session {
		s := model.Session{
			ID: id, ProjectID: pid, Title: title, FilePath: filepath.Join(t.TempDir(), id+".jsonl"),
			FileSize: size, FileMtime: mtime, MessageCount: 3, State: "ok",
		}
		if err := st.UpsertSession(ctx, s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	plain := mk("22222222-2222-4222-8222-222222222222", "未生成", 100)
	fresh := mk("33333333-3333-4333-8333-333333333333", "已生成", 200)
	stale := mk("44444444-4444-4444-8444-444444444444", "内容已变", 300)

	if err := st.UpsertAISummary(ctx, model.AISummary{
		SessionID: fresh.ID, Summary: "ok", Status: "ok", SourceMtime: mtime, SourceSize: 200,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertAISummary(ctx, model.AISummary{
		SessionID: stale.ID, Summary: "旧", Status: "ok", SourceMtime: mtime - 100, SourceSize: 299,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := st.PendingSummarySessions(ctx, pid, 2, 10, false, false)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, s := range got {
		ids[s.ID] = true
	}
	if !ids[plain.ID] || !ids[stale.ID] {
		t.Fatalf("应包含未生成与内容变化的会话: %+v", ids)
	}
	if ids[fresh.ID] {
		t.Fatalf("指纹一致的会话不应重复生成: %+v", ids)
	}

	// force 时全部重新入队。
	forced, err := st.PendingSummarySessions(ctx, pid, 2, 10, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(forced) != 3 {
		t.Fatalf("force 应返回 3 条，实际 %d", len(forced))
	}

	n, err := st.CountPendingSummaries(ctx, pid, 2)
	if err != nil || n != 2 {
		t.Fatalf("待生成计数不符: n=%d err=%v", n, err)
	}
}
