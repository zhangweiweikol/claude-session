// Author: zhangweiwei
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"claude-session/internal/model"
	"claude-session/internal/store"
)

func newAIServiceWithFake(t *testing.T, gen *fakeGenerator) (*Service, *store.Store, context.Context, int64) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "ai.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	pid, err := st.UpsertProject(ctx, t.TempDir(), "dir", "proj", time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	return New(st, gen, func(string) {}), st, ctx, pid
}

func addSessionsWithSummaries(t *testing.T, st *store.Store, ctx context.Context, pid int64, prefix string, n int) []string {
	t.Helper()
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%s-%03d", prefix, i)
		if err := st.UpsertSession(ctx, model.Session{
			ID: id, ProjectID: pid, Title: id,
			FilePath:     filepath.Join(t.TempDir(), id+".jsonl"),
			MessageCount: 2, State: "ok", FileMtime: 1,
		}); err != nil {
			t.Fatal(err)
		}
		addSummary(t, st, ctx, id)
		ids = append(ids, id)
	}
	return ids
}

func promptSessionIDs(prompt string) []string {
	var ids []string
	for _, line := range strings.Split(prompt, "\n") {
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		rest := strings.TrimPrefix(line, "- ")
		if i := strings.Index(rest, " |"); i > 0 {
			ids = append(ids, rest[:i])
		}
	}
	return ids
}

func categorizeReplyFor(folder string) func(req Request) (Result, error) {
	return func(req Request) (Result, error) {
		ids := promptSessionIDs(req.User)
		assignments := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			assignments = append(assignments, map[string]any{
				"session_id": id, "ref": 0, "new_folder": folder, "confidence": 0.9, "reason": "",
			})
		}
		raw, err := json.Marshal(map[string]any{
			"folders": []map[string]any{{
				"name": folder, "parent_ref": 0, "parent_name": "",
				"description": "", "confidence": 0.9,
			}},
			"assignments": assignments,
		})
		if err != nil {
			return Result{}, err
		}
		return Result{Structured: raw, Model: "fake", Profile: ProfileBare}, nil
	}
}

func TestCategorizeConcurrentMergesBatches(t *testing.T) {
	gen := &fakeGenerator{reply: categorizeReplyFor("批量类")}
	svc, st, ctx, pid := newAIServiceWithFake(t, gen)
	addSessionsWithSummaries(t, st, ctx, pid, "s", 80)

	sessions, err := st.CategorizeCandidates(ctx, pid, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 80 {
		t.Fatalf("candidates = %d, want 80", len(sessions))
	}

	jobID := "job-concurrent"
	if err := st.CreateAIJob(ctx, model.AIJob{
		ID: jobID, Kind: "categorize", ProjectID: pid, Status: "queued", Total: 80,
	}); err != nil {
		t.Fatal(err)
	}
	svc.processCategorize(jobID, pid, sessions, model.AISettings{})

	job, err := st.GetAIJob(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "done" || job.Done != 80 || job.Failed != 0 {
		t.Fatalf("job = %+v, want done=80 failed=0", job)
	}
	if gen.callCount() != 2 {
		t.Fatalf("80 个会话应拆成 2 批, calls = %d", gen.callCount())
	}
	list, err := st.ListSessionSuggestions(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 80 {
		t.Fatalf("suggestions = %d, want 80", len(list))
	}
	for _, a := range list {
		if a.NewFolder != "批量类" {
			t.Fatalf("会话 %s 应归入批量类, got %+v", a.SessionID, a)
		}
	}
}

func TestCategorizeBatchFailureDoesNotAffectOthers(t *testing.T) {
	gen := &fakeGenerator{reply: func(req Request) (Result, error) {
		if len(promptSessionIDs(req.User)) == 1 {
			return Result{}, &Error{Kind: KindUnavailable, Msg: "单批失败"}
		}
		return categorizeReplyFor("成功类")(req)
	}}
	svc, st, ctx, pid := newAIServiceWithFake(t, gen)
	addSessionsWithSummaries(t, st, ctx, pid, "s", 41)

	sessions, err := st.CategorizeCandidates(ctx, pid, false)
	if err != nil {
		t.Fatal(err)
	}
	jobID := "job-partial"
	if err := st.CreateAIJob(ctx, model.AIJob{
		ID: jobID, Kind: "categorize", ProjectID: pid, Status: "queued", Total: 41,
	}); err != nil {
		t.Fatal(err)
	}
	svc.processCategorize(jobID, pid, sessions, model.AISettings{})

	job, err := st.GetAIJob(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "done" || job.Done != 40 || job.Failed != 1 {
		t.Fatalf("job = %+v, want done=40 failed=1", job)
	}
	list, _ := st.ListSessionSuggestions(ctx, jobID)
	if len(list) != 41 {
		t.Fatalf("suggestions = %d, want 40 成功 + 1 keep", len(list))
	}
	kept := 0
	for _, a := range list {
		if a.Action == "keep" {
			kept++
		}
	}
	if kept != 1 {
		t.Errorf("失败批应补 1 条 keep 建议, got %d", kept)
	}
}

func TestCategorizeAllBatchesFail(t *testing.T) {
	gen := &fakeGenerator{reply: func(req Request) (Result, error) {
		return Result{}, &Error{Kind: KindUnavailable, Msg: "全部失败"}
	}}
	svc, st, ctx, pid := newAIServiceWithFake(t, gen)
	addSessionsWithSummaries(t, st, ctx, pid, "s", 3)

	sessions, _ := st.CategorizeCandidates(ctx, pid, false)
	jobID := "job-allfail"
	if err := st.CreateAIJob(ctx, model.AIJob{
		ID: jobID, Kind: "categorize", ProjectID: pid, Status: "queued", Total: 3,
	}); err != nil {
		t.Fatal(err)
	}
	svc.processCategorize(jobID, pid, sessions, model.AISettings{})

	job, err := st.GetAIJob(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "failed" || job.Done != 0 || job.Failed != 3 {
		t.Fatalf("job = %+v, want failed=3", job)
	}
}

func TestRunCategorizeBatchesKeepsCompletedOnTimeout(t *testing.T) {
	gen := &fakeGenerator{
		gate: func(ctx context.Context, req Request) {
			if strings.Contains(req.User, "slow") {
				<-ctx.Done()
			}
		},
		reply: func(req Request) (Result, error) {
			if strings.Contains(req.User, "slow") {
				return Result{}, &Error{Kind: KindTimeout, Msg: "超时"}
			}
			return categorizeReplyFor("快类")(req)
		},
	}
	svc, st, ctx, pid := newAIServiceWithFake(t, gen)
	jobID := "job-timeout"
	if err := st.CreateAIJob(ctx, model.AIJob{
		ID: jobID, Kind: "categorize", ProjectID: pid, Status: "queued", Total: 2,
	}); err != nil {
		t.Fatal(err)
	}

	batches := [][]CategorizeItem{
		{{SessionID: "fast", Title: "fast"}},
		{{SessionID: "slow", Title: "slow"}},
	}
	tctx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	results := svc.runCategorizeBatches(tctx, jobID, "proj", nil, batches, 2)

	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if results[0].err != nil || len(results[0].assignments) == 0 {
		t.Fatalf("快批应成功且保留结果, got %+v", results[0])
	}
	if results[1].err == nil {
		t.Fatalf("慢批应超时失败, got %+v", results[1])
	}
	job, err := st.GetAIJob(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Done != 1 || job.Failed != 1 {
		t.Fatalf("进度 = done:%d failed:%d, want 1/1", job.Done, job.Failed)
	}
}

func TestSummaryBatchDefaultAndClamp(t *testing.T) {
	if got := defaultSettings().SummaryBatchSize; got != 15 {
		t.Errorf("默认批量 = %d, want 15", got)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if err := st.SetSetting(ctx, settingsKey, `{"enabled":true,"summary_batch_size":0}`); err != nil {
		t.Fatal(err)
	}
	if got := LoadSettings(ctx, st).SummaryBatchSize; got != 15 {
		t.Errorf("0 应回退 15, got %d", got)
	}
	if err := st.SetSetting(ctx, settingsKey, `{"enabled":true,"summary_batch_size":99}`); err != nil {
		t.Fatal(err)
	}
	if got := LoadSettings(ctx, st).SummaryBatchSize; got != 20 {
		t.Errorf("超上限应钳制为 20, got %d", got)
	}
}

func TestSummaryEnqueueLimit(t *testing.T) {
	cfg := model.AISettings{AutoEnqueueLimit: 50}
	if got := summaryEnqueueLimit(cfg, false); got != 50 {
		t.Errorf("自动整理 = %d, want 50", got)
	}
	if got := summaryEnqueueLimit(cfg, true); got != manualEnqueueLimit {
		t.Errorf("手动整理 = %d, want %d", got, manualEnqueueLimit)
	}
	cfg.AutoEnqueueLimit = 300
	if got := summaryEnqueueLimit(cfg, true); got != 300 {
		t.Errorf("配置更高时应保留配置值, got %d", got)
	}
}

func TestDedupeFolderSuggestions(t *testing.T) {
	folders := []FolderSuggestionPayload{
		{Name: "重复", ParentRef: 0},
		{Name: " 重复 ", ParentRef: 0},
		{Name: "重复", ParentRef: 1},
	}
	got := dedupeFolderSuggestions(folders)
	if len(got) != 2 {
		t.Fatalf("去重后 = %d, want 2", len(got))
	}
}
