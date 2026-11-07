// Author: zhangweiwei
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"claude-session/internal/model"
	"claude-session/internal/store"
)

type fakeGenerator struct {
	calls int
	reply func(req Request) (Result, error)
}

func (f *fakeGenerator) Name() string { return "fake" }
func (f *fakeGenerator) Probe(ctx context.Context) (Profile, string, error) {
	return ProfileBare, "test", nil
}
func (f *fakeGenerator) Generate(ctx context.Context, req Request) (Result, error) {
	f.calls++
	if f.reply != nil {
		return f.reply(req)
	}
	return Result{Text: "{}", Structured: json.RawMessage(`{}`), Model: "fake", Profile: ProfileBare}, nil
}

func addSessionWithContent(t *testing.T, st *store.Store, ctx context.Context, pid int64, id, text string) model.Session {
	t.Helper()
	path := filepath.Join(t.TempDir(), id+".jsonl")
	lines := []string{
		fmt.Sprintf(`{"type":"user","uuid":"u-1","isSidechain":false,"origin":{"kind":"human"},"promptSource":"typed","message":{"role":"user","content":%q}}`, text),
		`{"type":"assistant","uuid":"a-1","isSidechain":false,"message":{"role":"assistant","content":[{"type":"text","text":"已处理"}]}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := model.Session{
		ID: id, ProjectID: pid, Title: id, FilePath: path,
		FileSize: 10, FileMtime: time.Now().Unix(), MessageCount: 2, State: "ok",
	}
	if err := st.UpsertSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	return sess
}

func newAIService(t *testing.T) (*Service, *store.Store, context.Context, int64) {
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
	return New(st, nil, func(string) {}), st, ctx, pid
}

func addSession(t *testing.T, st *store.Store, ctx context.Context, pid int64, id string) {
	t.Helper()
	if err := st.UpsertSession(ctx, model.Session{
		ID: id, ProjectID: pid, Title: id, FilePath: filepath.Join(t.TempDir(), id+".jsonl"),
		MessageCount: 2, State: "ok",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestApplySuggestionsCreatesFolders(t *testing.T) {
	svc, st, ctx, pid := newAIService(t)
	ids := []string{"a", "b", "c"}
	for _, id := range ids {
		addSession(t, st, ctx, pid, id)
	}

	jobID := "job-create"
	if err := st.CreateAIJob(ctx, model.AIJob{ID: jobID, Kind: "categorize", ProjectID: pid, Status: "done", Total: 3}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateFolderSuggestions(ctx, []store.FolderSuggestion{
		{JobID: jobID, Name: "后端"},
		{JobID: jobID, Name: "存储", ParentName: "后端", Description: "数据库与事务"},
	}); err != nil {
		t.Fatal(err)
	}
	var ss []store.SessionSuggestion
	for _, id := range ids {
		ss = append(ss, store.SessionSuggestion{
			JobID: jobID, SessionID: id, NewFolder: "存储",
			Action: "new_folder", Confidence: 0.9,
		})
	}
	if err := st.CreateSessionSuggestions(ctx, ss); err != nil {
		t.Fatal(err)
	}

	applied, err := svc.ApplySuggestions(ctx, jobID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 3 {
		t.Fatalf("应应用 3 个会话，实际 %d", applied)
	}
	folders, err := st.ListFolders(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 2 {
		t.Fatalf("应创建父级与子级两个归类，实际 %d: %+v", len(folders), folders)
	}
	for _, f := range folders {
		if f.Source != "ai" {
			t.Errorf("归类 %q 来源应为 ai，实际 %q", f.Name, f.Source)
		}
	}
	for _, id := range ids {
		fid, found, err := st.SessionFolder(ctx, id)
		if err != nil || !found || fid == nil {
			t.Fatalf("会话 %s 未归类: %v", id, err)
		}
	}
}

func TestApplySuggestionsRespectsThresholdAndConfidence(t *testing.T) {
	svc, st, ctx, pid := newAIService(t)
	addSession(t, st, ctx, pid, "solo")

	jobID := "job-threshold"
	if err := st.CreateAIJob(ctx, model.AIJob{ID: jobID, Kind: "categorize", ProjectID: pid, Status: "done", Total: 1}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateFolderSuggestions(ctx, []store.FolderSuggestion{
		{JobID: jobID, Name: "运维"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateSessionSuggestions(ctx, []store.SessionSuggestion{
		{JobID: jobID, SessionID: "solo", NewFolder: "运维", Action: "new_folder", Confidence: 0.9},
	}); err != nil {
		t.Fatal(err)
	}

	applied, err := svc.ApplySuggestions(ctx, jobID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 0 {
		t.Fatalf("单会话归类不应创建，实际应用 %d", applied)
	}
	folders, _ := st.ListFolders(ctx, pid)
	if len(folders) != 0 {
		t.Fatalf("不应创建归类: %+v", folders)
	}

	// 低置信度现在也允许应用（前端默认不勾选，用户可手动选中）。
	addSession(t, st, ctx, pid, "solo2")
	job2 := "job-low-confidence"
	if err := st.CreateAIJob(ctx, model.AIJob{ID: job2, Kind: "categorize", ProjectID: pid, Status: "done", Total: 2}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateFolderSuggestions(ctx, []store.FolderSuggestion{{JobID: job2, Name: "运维"}}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateSessionSuggestions(ctx, []store.SessionSuggestion{
		{JobID: job2, SessionID: "solo", NewFolder: "运维", Action: "new_folder", Confidence: 0.3},
		{JobID: job2, SessionID: "solo2", NewFolder: "运维", Action: "new_folder", Confidence: 0.3},
	}); err != nil {
		t.Fatal(err)
	}
	applied, err = svc.ApplySuggestions(ctx, job2, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 2 {
		t.Fatalf("低置信度现在允许应用，实际 %d", applied)
	}
}

func TestCategorizeCandidatesSkipsManualAndInherited(t *testing.T) {
	_, st, ctx, pid := newAIService(t)
	addSession(t, st, ctx, pid, "plain")
	addSession(t, st, ctx, pid, "manual")
	addSession(t, st, ctx, pid, "inherited")

	// 用一个空归类模拟已归类的会话。
	folder, err := st.CreateFolder(ctx, pid, "已有", nil, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	manualID := folder.ID
	if err := st.SetSessionFolder(ctx, "manual", &manualID); err != nil {
		t.Fatal(err)
	}
	if err := st.InheritFolderIfEmpty(ctx, "inherited", manualID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"plain", "manual", "inherited"} {
		if err := st.UpsertAISummary(ctx, model.AISummary{
			SessionID: id, Summary: "s", Status: "ok", SourceMtime: 1, SourceSize: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := st.CategorizeCandidates(ctx, pid, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "plain" {
		t.Fatalf("只应返回未归类会话，实际 %+v", got)
	}

	// reevaluate 时已由 AI 归类的会话参与，manual / inherited 仍排除。
	if err := st.SetSessionFolderAI(ctx, "plain", manualID); err != nil {
		t.Fatal(err)
	}
	got, err = st.CategorizeCandidates(ctx, pid, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "plain" {
		t.Fatalf("reevaluate 应包含 ai 归类会话，实际 %+v", got)
	}
}

func TestProcessBatchUsesSingleCall(t *testing.T) {
	_, st, ctx, pid := newAIService(t)
	var sessions []model.Session
	var items []string
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("batch-%d", i)
		sessions = append(sessions, addSessionWithContent(t, st, ctx, pid, id, fmt.Sprintf("问题 %d", i)))
		items = append(items, fmt.Sprintf(
			`{"session_id":%q,"summary":"摘要 %d","topics":["a"],"title":"标题 %d","confidence":0.9,"drift":0}`,
			id, i, i))
	}
	gen := &fakeGenerator{reply: func(req Request) (Result, error) {
		return Result{
			Structured: json.RawMessage(`{"items":[` + strings.Join(items, ",") + `]}`),
			Model:      "fake", Profile: ProfileBare,
		}, nil
	}}
	svc := New(st, gen, func(string) {})
	jobID := "job-batch"
	if err := st.CreateAIJob(ctx, model.AIJob{ID: jobID, Kind: "summary", ProjectID: pid, Status: "queued", Total: 3}); err != nil {
		t.Fatal(err)
	}

	svc.processBatch(summaryTask{jobID: jobID, sessions: sessions, force: true})

	if gen.calls != 1 {
		t.Fatalf("3 个会话应只调用一次模型，实际 %d", gen.calls)
	}
	for _, sess := range sessions {
		a, found, _ := st.GetAISummary(ctx, sess.ID)
		if !found || a.Status != "ok" || a.Summary == "" {
			t.Fatalf("会话 %s 概要未写入: %+v", sess.ID, a)
		}
	}
}

func TestProcessBatchRetriesMissingItems(t *testing.T) {
	_, st, ctx, pid := newAIService(t)
	sessions := []model.Session{
		addSessionWithContent(t, st, ctx, pid, "miss-1", "问题一"),
		addSessionWithContent(t, st, ctx, pid, "miss-2", "问题二"),
		addSessionWithContent(t, st, ctx, pid, "miss-3", "问题三"),
	}

	var gen *fakeGenerator
	gen = &fakeGenerator{reply: func(req Request) (Result, error) {
		if gen.calls == 1 {
			return Result{
				Structured: json.RawMessage(`{"items":[
					{"session_id":"miss-1","summary":"摘要一","topics":["a"],"title":"标题一","confidence":0.9,"drift":0},
					{"session_id":"miss-2","summary":"摘要二","topics":["a"],"title":"标题二","confidence":0.9,"drift":0}
				]}`),
				Model: "fake", Profile: ProfileBare,
			}, nil
		}
		return Result{
			Structured: json.RawMessage(`{"summary":"补生成","topics":["b"],"title":"补标题","confidence":0.8,"drift":0}`),
			Model:      "fake", Profile: ProfileBare,
		}, nil
	}}
	svc := New(st, gen, func(string) {})
	jobID := "job-miss"
	if err := st.CreateAIJob(ctx, model.AIJob{ID: jobID, Kind: "summary", ProjectID: pid, Status: "queued", Total: 3}); err != nil {
		t.Fatal(err)
	}

	svc.processBatch(summaryTask{jobID: jobID, sessions: sessions, force: true})

	if gen.calls != 2 {
		t.Fatalf("批量缺项应触发一次单条重试，实际调用 %d 次", gen.calls)
	}
	missing, found, _ := st.GetAISummary(ctx, "miss-3")
	if !found || missing.Summary != "补生成" {
		t.Fatalf("缺失会话未补生成: %+v", missing)
	}
}

func TestApplySuggestionsNormalizesFolderNames(t *testing.T) {
	svc, st, ctx, pid := newAIService(t)
	addSession(t, st, ctx, pid, "norm-1")
	addSession(t, st, ctx, pid, "norm-2")

	jobID := "job-norm"
	if err := st.CreateAIJob(ctx, model.AIJob{ID: jobID, Kind: "categorize", ProjectID: pid, Status: "done", Total: 2}); err != nil {
		t.Fatal(err)
	}
	// 归类建议名称带空格，会话引用去掉了空格。
	if err := st.CreateFolderSuggestions(ctx, []store.FolderSuggestion{
		{JobID: jobID, Name: "Docker 部署", Description: "镜像与编排"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateSessionSuggestions(ctx, []store.SessionSuggestion{
		{JobID: jobID, SessionID: "norm-1", NewFolder: "Docker部署", Action: "new_folder", Confidence: 0.9},
		{JobID: jobID, SessionID: "norm-2", NewFolder: "docker 部署", Action: "new_folder", Confidence: 0.9},
	}); err != nil {
		t.Fatal(err)
	}

	applied, err := svc.ApplySuggestions(ctx, jobID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 2 {
		t.Fatalf("名称归一化后应应用 2 个会话，实际 %d", applied)
	}
	folders, _ := st.ListFolders(ctx, pid)
	if len(folders) != 1 || folders[0].Name != "Docker 部署" {
		t.Fatalf("只应创建一个规范名称的归类: %+v", folders)
	}
}

func TestApplySuggestionsReusesExistingFolder(t *testing.T) {
	svc, st, ctx, pid := newAIService(t)
	addSession(t, st, ctx, pid, "reuse-1")
	addSession(t, st, ctx, pid, "reuse-2")

	parent, err := st.CreateFolder(ctx, pid, "开发", nil, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	child, err := st.CreateFolder(ctx, pid, "SpringBoot", &parent.ID, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}

	jobID := "job-reuse"
	if err := st.CreateAIJob(ctx, model.AIJob{ID: jobID, Kind: "categorize", ProjectID: pid, Status: "done", Total: 2}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateSessionSuggestions(ctx, []store.SessionSuggestion{
		{JobID: jobID, SessionID: "reuse-1", FolderID: child.ID, Action: "reuse", Confidence: 0.9},
		{JobID: jobID, SessionID: "reuse-2", FolderID: child.ID, Action: "reuse", Confidence: 0.9},
	}); err != nil {
		t.Fatal(err)
	}

	applied, err := svc.ApplySuggestions(ctx, jobID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 2 {
		t.Fatalf("应复用已有归类并应用 2 个会话，实际 %d", applied)
	}
	folders, _ := st.ListFolders(ctx, pid)
	if len(folders) != 2 {
		t.Fatalf("复用不应创建新归类，实际 %d", len(folders))
	}
	for _, id := range []string{"reuse-1", "reuse-2"} {
		fid, _, _ := st.SessionFolder(ctx, id)
		if fid == nil || *fid != child.ID {
			t.Fatalf("会话 %s 应归入已有子归类", id)
		}
	}
}

func TestApplySuggestionsCreatesChildUnderExisting(t *testing.T) {
	svc, st, ctx, pid := newAIService(t)
	addSession(t, st, ctx, pid, "child-1")
	addSession(t, st, ctx, pid, "child-2")

	parent, err := st.CreateFolder(ctx, pid, "开发", nil, time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}

	jobID := "job-child"
	if err := st.CreateAIJob(ctx, model.AIJob{ID: jobID, Kind: "categorize", ProjectID: pid, Status: "done", Total: 2}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateFolderSuggestions(ctx, []store.FolderSuggestion{
		{JobID: jobID, Name: "SpringBoot", ParentID: parent.ID, Description: "启动与依赖"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateSessionSuggestions(ctx, []store.SessionSuggestion{
		{JobID: jobID, SessionID: "child-1", NewFolder: "SpringBoot", Action: "new_folder", Confidence: 0.9},
		{JobID: jobID, SessionID: "child-2", NewFolder: "SpringBoot", Action: "new_folder", Confidence: 0.9},
	}); err != nil {
		t.Fatal(err)
	}

	applied, err := svc.ApplySuggestions(ctx, jobID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 2 {
		t.Fatalf("应应用 2 个会话，实际 %d", applied)
	}
	folders, _ := st.ListFolders(ctx, pid)
	if len(folders) != 2 {
		t.Fatalf("应创建 1 个二级归类，实际总数 %d", len(folders))
	}
	var child *model.Folder
	for i := range folders {
		if folders[i].Name == "SpringBoot" {
			child = &folders[i]
		}
	}
	if child == nil || child.ParentID == nil || *child.ParentID != parent.ID {
		t.Fatalf("新归类应挂在已有「开发」下: %+v", folders)
	}
}

func TestCapTopFolders(t *testing.T) {
	var folders []FolderSuggestionPayload
	var assignments []AssignmentPayload
	for i := 0; i < 12; i++ {
		name := fmt.Sprintf("类%d", i)
		folders = append(folders, FolderSuggestionPayload{Name: name, Confidence: 0.8})
		assignments = append(assignments,
			AssignmentPayload{SessionID: fmt.Sprintf("s%d-a", i), NewFolder: name, Confidence: 0.9},
			AssignmentPayload{SessionID: fmt.Sprintf("s%d-b", i), NewFolder: name, Confidence: 0.9},
		)
	}

	kept, out := capTopFolders(folders, assignments, nil)
	top := 0
	hasOther := false
	for _, f := range kept {
		if f.ParentRef == 0 && f.ParentName == "" {
			top++
		}
		if f.Name == "其他" {
			hasOther = true
		}
	}
	if top != 10 {
		t.Fatalf("顶级归类应为 9 个新类 + 其他，实际 %d", top)
	}
	if !hasOther {
		t.Fatal("应创建「其他」归类")
	}
	merged := 0
	for _, a := range out {
		if a.NewFolder == "其他" {
			merged++
		}
	}
	if merged != 6 {
		t.Fatalf("应合并 3 个归类共 6 个会话到其他，实际 %d", merged)
	}
}

func TestCapTopFoldersRemovesUnneededOther(t *testing.T) {
	folders := []FolderSuggestionPayload{
		{Name: "开发", Confidence: 0.9},
		{Name: "运维", Confidence: 0.9},
		{Name: "其他", Confidence: 0.5},
	}
	assignments := []AssignmentPayload{
		{SessionID: "s1", NewFolder: "开发", Confidence: 0.9},
		{SessionID: "s2", NewFolder: "运维", Confidence: 0.9},
		{SessionID: "s3", NewFolder: "其他", Confidence: 0.4},
	}

	kept, out := capTopFolders(folders, assignments, nil)
	for _, f := range kept {
		if f.Name == "其他" {
			t.Fatal("未超过 9 个顶级归类时不应保留「其他」")
		}
	}
	for _, a := range out {
		if a.SessionID == "s3" && a.NewFolder != "" {
			t.Fatalf("归入「其他」的会话应改为保持未归类: %+v", a)
		}
	}
}

func TestSortByLineage(t *testing.T) {
	list := []model.Session{
		{ID: "child", ParentID: "parent"},
		{ID: "grand", ParentID: "child"},
		{ID: "parent"},
		{ID: "cyclic-a", ParentID: "cyclic-b"},
		{ID: "cyclic-b", ParentID: "cyclic-a"},
	}
	sortByLineage(list)

	pos := map[string]int{}
	for i, s := range list {
		pos[s.ID] = i
	}
	if pos["parent"] > pos["child"] {
		t.Fatalf("父会话应排在子会话之前: %+v", pos)
	}
	if pos["child"] > pos["grand"] {
		t.Fatalf("子会话应排在孙会话之前: %+v", pos)
	}
}

func TestBuildSummaryRequestWithParent(t *testing.T) {
	req := buildSummaryRequest(model.Session{ID: "s", Title: "子会话"}, "proj", "[用户] 新问题", "父会话已完成锁冲突排查")
	if !strings.Contains(req.User, "[父会话概要]") || !strings.Contains(req.User, "drift") {
		t.Fatalf("派生会话 prompt 缺少父摘要或 drift 要求:\n%s", req.User)
	}
	plain := buildSummaryRequest(model.Session{ID: "s", Title: "普通会话"}, "proj", "[用户] 问题", "")
	if strings.Contains(plain.User, "[父会话概要]") {
		t.Fatalf("普通会话不应包含父摘要:\n%s", plain.User)
	}
	if !strings.Contains(string(plain.Schema), `"drift"`) {
		t.Fatal("schema 应包含 drift")
	}
}

func TestPathNormalize(t *testing.T) {
	cases := map[string]string{
		"后端":      "后端",
		" 后端 ":    "后端",
		"ＡＰＩ":     "api",
		"接口/文档":   "接口文档",
		"a-b_c d": "abcd",
	}
	for in, want := range cases {
		if got := normalizeName(in); got != want {
			t.Errorf("normalizeName(%q) = %q, want %q", in, got, want)
		}
	}
	if pathKey([]string{"后端", "存储"}) != pathKey([]string{" 后端 ", "存储"}) {
		t.Error("路径归一化后应相等")
	}
}
