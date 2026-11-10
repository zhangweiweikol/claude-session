// Author: zhangweiwei
package ai

import (
	"fmt"
	"sort"
	"strings"

	"claude-session/internal/model"
	"claude-session/internal/store"
)

// lineageNode 是归类分组用的会话节点。
type lineageNode struct {
	ID        string
	ParentID  string
	FolderID  *int64
	Source    string
	Candidate bool
}

// lineageAssignment 是模型给出的归类目标，Ref 指向已有归类编号，NewFolder 表示本批新建。
type lineageAssignment struct {
	Ref        int
	NewFolder  string
	Confidence float64
	Reason     string
}

func (a lineageAssignment) isEmpty() bool {
	return a.Ref == 0 && strings.TrimSpace(a.NewFolder) == ""
}

func (a lineageAssignment) key() string {
	if a.Ref > 0 {
		return fmt.Sprintf("r:%d", a.Ref)
	}
	return "n:" + normalizeName(a.NewFolder)
}

func lineageNodesFrom(states map[string]store.SessionLineage, candidates map[string]bool) map[string]lineageNode {
	out := make(map[string]lineageNode, len(states))
	for id, s := range states {
		out[id] = lineageNode{
			ID: id, ParentID: s.ParentID, FolderID: s.FolderID,
			Source: s.FolderSource, Candidate: candidates[id],
		}
	}
	return out
}

// buildLineageGroups 按 parent_id 把会话连成连通分量，只返回包含候选会话的组。
// 成员按到根的深度升序排列，父会话在前。
func buildLineageGroups(nodes map[string]lineageNode, candidates map[string]bool) [][]string {
	parent := make(map[string]string, len(nodes))
	for id := range nodes {
		parent[id] = id
	}
	var find func(string) string
	find = func(x string) string {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	for id, n := range nodes {
		if n.ParentID == "" {
			continue
		}
		if _, ok := nodes[n.ParentID]; !ok {
			continue
		}
		ra, rb := find(id), find(n.ParentID)
		if ra != rb {
			parent[ra] = rb
		}
	}

	grouped := make(map[string][]string)
	for id := range nodes {
		root := find(id)
		grouped[root] = append(grouped[root], id)
	}

	var groups [][]string
	for _, members := range grouped {
		hasCandidate := false
		for _, id := range members {
			if candidates[id] {
				hasCandidate = true
				break
			}
		}
		if !hasCandidate {
			continue
		}
		depth := make(map[string]int, len(members))
		for _, id := range members {
			depth[id] = lineageDepth(id, nodes)
		}
		sort.Slice(members, func(i, j int) bool {
			if depth[members[i]] != depth[members[j]] {
				return depth[members[i]] < depth[members[j]]
			}
			return members[i] < members[j]
		})
		groups = append(groups, members)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i][0] < groups[j][0] })
	return groups
}

// lineageDepth 返回会话到谱系根的深度，父缺失或成环时停止。
func lineageDepth(id string, nodes map[string]lineageNode) int {
	seen := map[string]bool{id: true}
	depth := 0
	cur := nodes[id]
	for cur.ParentID != "" {
		p, ok := nodes[cur.ParentID]
		if !ok || seen[cur.ParentID] {
			break
		}
		seen[cur.ParentID] = true
		depth++
		cur = p
	}
	return depth
}

// planCategorizeBatches 把多个谱系组装进不超过 limit 的批次。
// 组内候选会话保持连续，只有单个组自身超过 limit 时才拆组；
// 只为可见候选数 >= 2 的组分配谱系标签，单元素组不标注。
func planCategorizeBatches(groups [][]string, candidates map[string]bool, limit int) ([][]string, map[string]string) {
	if limit <= 0 {
		limit = categorizeBatch
	}
	labels := make(map[string]string)
	var blocks [][]string
	labelSeq := 0
	for _, members := range groups {
		visible := make([]string, 0, len(members))
		for _, id := range members {
			if candidates[id] {
				visible = append(visible, id)
			}
		}
		if len(visible) == 0 {
			continue
		}
		if len(visible) >= 2 {
			labelSeq++
			label := fmt.Sprintf("G%d", labelSeq)
			for _, id := range visible {
				labels[id] = label
			}
		}
		for start := 0; start < len(visible); start += limit {
			end := start + limit
			if end > len(visible) {
				end = len(visible)
			}
			blocks = append(blocks, visible[start:end])
		}
	}

	var packs [][]string
	var current []string
	for _, block := range blocks {
		if len(current) > 0 && len(current)+len(block) > limit {
			packs = append(packs, current)
			current = nil
		}
		current = append(current, block...)
		if len(current) >= limit {
			packs = append(packs, current)
			current = nil
		}
	}
	if len(current) > 0 {
		packs = append(packs, current)
	}
	return packs, labels
}

// pickGroupTarget 按优先级为整组选择归类：
// 1. 组内 manual 归类（离根最近）
// 2. 组内不参与本次整理的已归类会话（父会话已归类）
// 3. 候选建议的多数票
func pickGroupTarget(members []string, nodes map[string]lineageNode, assignments map[string]lineageAssignment, folderRef map[int64]int) (lineageAssignment, bool) {
	for _, id := range members {
		n := nodes[id]
		if n.FolderID == nil || n.Source != "manual" {
			continue
		}
		if ref, ok := folderRef[*n.FolderID]; ok {
			return lineageAssignment{Ref: ref}, true
		}
	}
	for _, id := range members {
		n := nodes[id]
		if n.Candidate || n.FolderID == nil {
			continue
		}
		if ref, ok := folderRef[*n.FolderID]; ok {
			return lineageAssignment{Ref: ref}, true
		}
	}

	type vote struct {
		count int
		conf  float64
		a     lineageAssignment
	}
	votes := make(map[string]*vote)
	rootKey := ""
	for _, id := range members {
		if !nodes[id].Candidate {
			continue
		}
		a, ok := assignments[id]
		if !ok || a.isEmpty() {
			continue
		}
		k := a.key()
		v := votes[k]
		if v == nil {
			v = &vote{a: a}
			votes[k] = v
		}
		v.count++
		v.conf += a.Confidence
		if rootKey == "" {
			rootKey = k
		}
	}
	if len(votes) == 0 {
		return lineageAssignment{}, false
	}

	bestKey := ""
	for k, v := range votes {
		if bestKey == "" {
			bestKey = k
			continue
		}
		b := votes[bestKey]
		if v.count > b.count || (v.count == b.count && v.conf > b.conf) {
			bestKey = k
		}
	}
	// 票数和置信度都相同时，优先离根最近的候选建议。
	if v, ok := votes[rootKey]; ok {
		b := votes[bestKey]
		if v.count == b.count && v.conf == b.conf {
			bestKey = rootKey
		}
	}
	return votes[bestKey].a, true
}

// applyLineageTargets 把每个谱系组的候选会话统一到同一个归类目标。
func applyLineageTargets(groups [][]string, nodes map[string]lineageNode, assignments map[string]lineageAssignment, folderRef map[int64]int) {
	for _, members := range groups {
		target, ok := pickGroupTarget(members, nodes, assignments, folderRef)
		if !ok {
			continue
		}
		for _, id := range members {
			if !nodes[id].Candidate {
				continue
			}
			prev := assignments[id]
			next := target
			if prev.Confidence > 0 {
				next.Confidence = prev.Confidence
			} else if next.Confidence == 0 {
				next.Confidence = 0.9
			}
			next.Reason = prev.Reason
			assignments[id] = next
		}
	}
}

// expandLineageCandidates 补回候选所在谱系里其他可整理会话，避免分批截断谱系。
func expandLineageCandidates(candidates []model.Session, states map[string]store.SessionLineage, reevaluate bool) []string {
	known := make(map[string]bool, len(candidates))
	for _, sess := range candidates {
		known[sess.ID] = true
	}
	groups := buildLineageGroups(lineageNodesFrom(states, known), known)
	var extra []string
	for _, members := range groups {
		for _, id := range members {
			if known[id] || !canCategorize(states[id], reevaluate) {
				continue
			}
			extra = append(extra, id)
			known[id] = true
		}
	}
	return extra
}

func canCategorize(s store.SessionLineage, reevaluate bool) bool {
	if s.State != "ok" || s.DeletedAt > 0 || !s.HasSummary {
		return false
	}
	if s.FolderID == nil && s.FolderSource == "" {
		return true
	}
	return reevaluate && s.FolderSource == "ai"
}
