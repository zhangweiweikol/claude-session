// Author: zhangweiwei
package ai

import (
	"fmt"
	"testing"

	"claude-session/internal/model"
	"claude-session/internal/store"
)

func folderPtr(v int64) *int64 { return &v }

func groupWith(groups [][]string, id string) []string {
	for _, members := range groups {
		for _, m := range members {
			if m == id {
				return members
			}
		}
	}
	return nil
}

func TestBuildLineageGroups(t *testing.T) {
	nodes := map[string]lineageNode{
		"root":  {ID: "root"},
		"child": {ID: "child", ParentID: "root"},
		"grand": {ID: "grand", ParentID: "child"},
		"other": {ID: "other"},
		"cyc-a": {ID: "cyc-a", ParentID: "cyc-b"},
		"cyc-b": {ID: "cyc-b", ParentID: "cyc-a"},
	}
	candidates := map[string]bool{"grand": true, "other": true, "cyc-a": true}
	groups := buildLineageGroups(nodes, candidates)

	chain := groupWith(groups, "grand")
	if len(chain) != 3 || chain[0] != "root" || chain[1] != "child" || chain[2] != "grand" {
		t.Errorf("谱系组 = %v, want [root child grand]", chain)
	}
	if g := groupWith(groups, "other"); len(g) != 1 {
		t.Errorf("独立会话应自成一组, got %v", g)
	}
	if g := groupWith(groups, "cyc-a"); len(g) != 2 {
		t.Errorf("环应合并为一组, got %v", g)
	}
}

func TestPickGroupTargetPrefersManual(t *testing.T) {
	nodes := map[string]lineageNode{
		"root":  {ID: "root", FolderID: folderPtr(7), Source: "manual"},
		"child": {ID: "child", ParentID: "root", Candidate: true},
	}
	assignments := map[string]lineageAssignment{
		"child": {Ref: 2, Confidence: 0.9},
	}
	target, ok := pickGroupTarget([]string{"root", "child"}, nodes, assignments, map[int64]int{7: 1})
	if !ok || target.Ref != 1 {
		t.Fatalf("应跟随 manual 父归类, got %+v ok=%v", target, ok)
	}
}

func TestPickGroupTargetFollowsCategorizedParent(t *testing.T) {
	nodes := map[string]lineageNode{
		"root":  {ID: "root", FolderID: folderPtr(7), Source: "ai"},
		"child": {ID: "child", ParentID: "root", Candidate: true},
	}
	assignments := map[string]lineageAssignment{
		"child": {Ref: 2, Confidence: 0.9},
	}
	target, ok := pickGroupTarget([]string{"root", "child"}, nodes, assignments, map[int64]int{7: 1})
	if !ok || target.Ref != 1 {
		t.Fatalf("应跟随已归类的父会话, got %+v ok=%v", target, ok)
	}
}

func TestPickGroupTargetMajority(t *testing.T) {
	nodes := map[string]lineageNode{
		"a": {ID: "a", Candidate: true},
		"b": {ID: "b", ParentID: "a", Candidate: true},
		"c": {ID: "c", ParentID: "b", Candidate: true},
	}
	assignments := map[string]lineageAssignment{
		"a": {Ref: 2, Confidence: 0.5},
		"b": {Ref: 2, Confidence: 0.5},
		"c": {NewFolder: "新类", Confidence: 0.6},
	}
	target, ok := pickGroupTarget([]string{"a", "b", "c"}, nodes, assignments, map[int64]int{})
	if !ok || target.Ref != 2 {
		t.Fatalf("应取多数票, got %+v ok=%v", target, ok)
	}
}

func TestPickGroupTargetTiePrefersRoot(t *testing.T) {
	nodes := map[string]lineageNode{
		"a": {ID: "a", Candidate: true},
		"b": {ID: "b", ParentID: "a", Candidate: true},
	}
	assignments := map[string]lineageAssignment{
		"a": {Ref: 1, Confidence: 0.5},
		"b": {Ref: 2, Confidence: 0.5},
	}
	target, ok := pickGroupTarget([]string{"a", "b"}, nodes, assignments, map[int64]int{})
	if !ok || target.Ref != 1 {
		t.Fatalf("平票应取根会话建议, got %+v ok=%v", target, ok)
	}
}

func TestApplyLineageTargetsUnifiesGroup(t *testing.T) {
	nodes := map[string]lineageNode{
		"root":  {ID: "root", Candidate: true},
		"child": {ID: "child", ParentID: "root", Candidate: true},
		"leaf":  {ID: "leaf", ParentID: "child", Candidate: true},
	}
	assignments := map[string]lineageAssignment{
		"root":  {Ref: 1, Confidence: 0.7},
		"child": {Ref: 2, Confidence: 0.6},
	}
	applyLineageTargets([][]string{{"root", "child", "leaf"}}, nodes, assignments, map[int64]int{})

	for _, id := range []string{"root", "child", "leaf"} {
		if assignments[id].Ref != 1 {
			t.Errorf("%s 应统一到根建议, got %+v", id, assignments[id])
		}
	}
	if assignments["leaf"].Confidence == 0 {
		t.Errorf("模型遗漏的成员应有默认置信度")
	}
}

func TestExpandLineageCandidates(t *testing.T) {
	states := map[string]store.SessionLineage{
		"root":  {State: "ok", HasSummary: true},
		"child": {ParentID: "root", State: "ok", HasSummary: true},
		"leaf":  {ParentID: "child", State: "ok", HasSummary: true},
		"manual": {ParentID: "root", State: "ok", HasSummary: true, FolderSource: "manual",
			FolderID: folderPtr(3)},
	}
	candidates := []model.Session{{ID: "child"}}
	extra := expandLineageCandidates(candidates, states, false)

	found := map[string]bool{}
	for _, id := range extra {
		found[id] = true
	}
	if !found["root"] || !found["leaf"] {
		t.Errorf("应补回同谱系未归类会话, got %v", extra)
	}
	if found["manual"] {
		t.Errorf("已手动归类的会话不应补回, got %v", extra)
	}
}

func TestPlanCategorizeBatchesMergesSingletonGroups(t *testing.T) {
	groups := [][]string{{"root", "child"}, {"solo1"}, {"solo2"}, {"solo3"}}
	candidates := map[string]bool{
		"root": true, "child": true, "solo1": true, "solo2": true, "solo3": true,
	}
	packs, labels := planCategorizeBatches(groups, candidates, 40)
	if len(packs) != 1 {
		t.Fatalf("多个组应共享同一批次, got %v", packs)
	}
	if len(packs[0]) != 5 {
		t.Fatalf("批次应包含全部候选, got %v", packs[0])
	}
	if labels["root"] != "G1" || labels["child"] != "G1" {
		t.Errorf("谱系组应带 G1 标签, got %v", labels)
	}
	for _, id := range []string{"solo1", "solo2", "solo3"} {
		if labels[id] != "" {
			t.Errorf("单元素组不应带谱系标签, %s=%q", id, labels[id])
		}
	}
}

func TestPlanCategorizeBatchesKeepsGroupAtomic(t *testing.T) {
	groups := [][]string{{"a", "b"}, {"c", "d"}}
	candidates := map[string]bool{"a": true, "b": true, "c": true, "d": true}
	packs, _ := planCategorizeBatches(groups, candidates, 3)
	if len(packs) != 2 {
		t.Fatalf("每组应各占一批, got %v", packs)
	}
	if len(packs[0]) != 2 || packs[0][0] != "a" || packs[0][1] != "b" {
		t.Errorf("第一批应按组完整保留, got %v", packs[0])
	}
	if len(packs[1]) != 2 || packs[1][0] != "c" || packs[1][1] != "d" {
		t.Errorf("第二批应按组完整保留, got %v", packs[1])
	}
}

func TestPlanCategorizeBatchesSplitsOversizedGroup(t *testing.T) {
	groups := [][]string{{"a", "b", "c", "d", "e"}, {"solo"}}
	candidates := map[string]bool{
		"a": true, "b": true, "c": true, "d": true, "e": true, "solo": true,
	}
	packs, labels := planCategorizeBatches(groups, candidates, 2)
	want := [][]string{{"a", "b"}, {"c", "d"}, {"e", "solo"}}
	if len(packs) != len(want) {
		t.Fatalf("批次 = %v, want %v", packs, want)
	}
	for i := range want {
		if len(packs[i]) != len(want[i]) {
			t.Fatalf("批次 %d = %v, want %v", i, packs[i], want[i])
		}
		for j := range want[i] {
			if packs[i][j] != want[i][j] {
				t.Fatalf("批次 %d = %v, want %v", i, packs[i], want[i])
			}
		}
	}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		if labels[id] != "G1" {
			t.Errorf("超大批次成员应保持同一标签, %s=%q", id, labels[id])
		}
	}
	if labels["solo"] != "" {
		t.Errorf("单元素组不应带标签, got %q", labels["solo"])
	}
}

func TestPlanCategorizeBatchesIgnoresNonCandidates(t *testing.T) {
	groups := [][]string{{"parent", "child"}}
	candidates := map[string]bool{"child": true}
	packs, labels := planCategorizeBatches(groups, candidates, 40)
	if len(packs) != 1 || len(packs[0]) != 1 || packs[0][0] != "child" {
		t.Fatalf("只应包含候选会话, got %v", packs)
	}
	if labels["child"] != "" {
		t.Errorf("可见候选只有 1 个时不应带标签, got %q", labels["child"])
	}
}

func TestPlanCategorizeBatchesRespectsLimit(t *testing.T) {
	groups := [][]string{{"root", "c1", "c2", "c3", "c4"}}
	candidates := map[string]bool{"root": true, "c1": true, "c2": true, "c3": true, "c4": true}
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("solo-%03d", i)
		groups = append(groups, []string{id})
		candidates[id] = true
	}
	packs, _ := planCategorizeBatches(groups, candidates, 40)

	seen := map[string]int{}
	for _, pack := range packs {
		if len(pack) > 40 {
			t.Fatalf("批次超过上限: %d", len(pack))
		}
		for _, id := range pack {
			seen[id]++
		}
	}
	if len(seen) != len(candidates) {
		t.Fatalf("候选覆盖 = %d, want %d", len(seen), len(candidates))
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("%s 出现 %d 次, want 1", id, n)
		}
	}
}
