// Author: zhangweiwei
package ai

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"claude-session/internal/model"
	"claude-session/internal/scanner"
	"claude-session/internal/store"
)

const (
	maxAttempts = 3
	jobQueueCap = 500
)

var retryBackoff = []time.Duration{5 * time.Second, 20 * time.Second, 60 * time.Second}

type summaryTask struct {
	jobID    string
	sessions []model.Session
	force    bool
}

// preparedSummary 是完成采样、可直接请求模型的会话。
type preparedSummary struct {
	sess        model.Session
	prev        model.AISummary
	sample      string
	parent      string
	lineageMode string
}

type Service struct {
	st        *store.Store
	gen       Generator
	broadcast func(string)
	queue     chan summaryTask

	mu        sync.RWMutex
	probeMu   sync.Mutex
	probed    bool
	available bool
	profile   Profile
	version   string
	model     string
	probeErr  string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func New(st *store.Store, gen Generator, broadcast func(string)) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		st: st, gen: gen, broadcast: broadcast,
		queue: make(chan summaryTask, jobQueueCap),
		ctx:   ctx, cancel: cancel,
	}
}

func (s *Service) Generator() Generator { return s.gen }

// Start 启动 worker。服务重启后把中断的 running 状态恢复为待处理。
func (s *Service) Start() {
	if err := s.st.ResetRunningSummaries(s.ctx); err != nil {
		log.Printf("恢复 AI 任务状态失败: %v", err)
	}
	if err := s.st.ResetActiveJobs(s.ctx); err != nil {
		log.Printf("清理中断的 AI 任务失败: %v", err)
	}
	for i := 0; i < 2; i++ {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.worker()
		}()
	}
}

func (s *Service) Stop() {
	s.cancel()
	s.wg.Wait()
}

// Probe 探测可用档位并缓存结果。首次手动操作时自动执行，也可由设置页手动触发。
func (s *Service) Probe(ctx context.Context) error {
	profile, version, err := s.gen.Probe(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.version = version
	s.probed = true
	if err != nil {
		s.available = false
		s.probeErr = err.Error()
		return err
	}
	s.available = true
	s.profile = profile
	s.probeErr = ""
	if cli, ok := s.gen.(*CLIGenerator); ok {
		s.model = cli.model
	}
	return nil
}

// ensureProbed 在首次使用时探测能力，之后复用结果。
func (s *Service) ensureProbed(ctx context.Context) error {
	s.probeMu.Lock()
	defer s.probeMu.Unlock()

	s.mu.RLock()
	probed, available, msg := s.probed, s.available, s.probeErr
	s.mu.RUnlock()
	if probed {
		if available {
			return nil
		}
		if msg == "" {
			msg = "AI 功能不可用"
		}
		return errors.New(msg)
	}

	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	return s.Probe(probeCtx)
}

func (s *Service) Available() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.available
}

// ApplyModel 在设置变更后同步生成器使用的模型名。
func (s *Service) ApplyModel(name string) {
	if setter, ok := s.gen.(interface{ SetModel(string) }); ok {
		setter.SetModel(name)
	}
	s.setModel(name)
}

func (s *Service) setModel(name string) {
	if name == "" {
		return
	}
	s.mu.Lock()
	s.model = name
	s.mu.Unlock()
}

// Enqueue 按项目入队待生成概要的会话。手动触发时跳过 60 秒写入保护。
func (s *Service) Enqueue(ctx context.Context, projectID int64, force, manual bool) (string, int, error) {
	if err := s.ensureProbed(ctx); err != nil {
		return "", 0, err
	}
	cfg := LoadSettings(ctx, s.st)
	if !cfg.Enabled {
		return "", 0, errors.New("AI 功能已关闭")
	}

	if id, ok, err := s.st.FindActiveJob(ctx, "summary", projectID); err == nil && ok {
		return id, 0, nil
	}

	project, err := s.st.GetProject(ctx, projectID)
	if err != nil {
		return "", 0, errors.New("项目不存在")
	}
	if excluded(cfg, project.Cwd) {
		return "", 0, errors.New("该项目已加入 AI 排除名单")
	}

	sessions, err := s.st.PendingSummarySessions(ctx, projectID, cfg.MinMessages, cfg.AutoEnqueueLimit, force, manual)
	if err != nil {
		return "", 0, err
	}
	// 自动触发且没有候选时不创建空任务，避免每 30 秒扫描都写一条 job。
	if len(sessions) == 0 && !manual {
		return "", 0, nil
	}
	return s.enqueueSessions(ctx, projectID, sessions, force)
}

// EnqueueIDs 为指定会话生成概要，用于详情页「重新生成」。
func (s *Service) EnqueueIDs(ctx context.Context, projectID int64, ids []string, force bool) (string, int, error) {
	if err := s.ensureProbed(ctx); err != nil {
		return "", 0, err
	}
	cfg := LoadSettings(ctx, s.st)
	if !cfg.Enabled {
		return "", 0, errors.New("AI 功能已关闭")
	}
	all, err := s.st.GetSessionsByIDs(ctx, ids)
	if err != nil {
		return "", 0, err
	}
	sessions := make([]model.Session, 0, len(all))
	for _, sess := range all {
		if sess.State != "ok" || sess.DeletedAt > 0 || sess.MessageCount < cfg.MinMessages {
			continue
		}
		sessions = append(sessions, sess)
	}
	return s.enqueueSessions(ctx, projectID, sessions, force)
}

func (s *Service) enqueueSessions(ctx context.Context, projectID int64, sessions []model.Session, force bool) (string, int, error) {
	jobID := newID()
	now := time.Now().Unix()
	job := model.AIJob{
		ID: jobID, Kind: "summary", ProjectID: projectID,
		Status: "queued", Total: len(sessions), CreatedAt: now, UpdatedAt: now,
	}
	if err := s.st.CreateAIJob(ctx, job); err != nil {
		return "", 0, err
	}
	if len(sessions) == 0 {
		_ = s.st.FinishAIJobIfComplete(ctx, jobID)
		return jobID, 0, nil
	}
	// 父会话先处理，子会话才能复用父摘要做增量。
	sortByLineage(sessions)

	batchSize := LoadSettings(ctx, s.st).SummaryBatchSize
	if batchSize <= 0 {
		batchSize = 10
	}
	for start := 0; start < len(sessions); start += batchSize {
		end := start + batchSize
		if end > len(sessions) {
			end = len(sessions)
		}
		select {
		case s.queue <- summaryTask{jobID: jobID, sessions: sessions[start:end], force: force}:
		case <-s.ctx.Done():
			return jobID, 0, s.ctx.Err()
		}
	}
	s.broadcast("ai")
	return jobID, len(sessions), nil
}

func (s *Service) worker() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case task := <-s.queue:
			s.processBatch(task)
		}
	}
}

func (s *Service) processBatch(task summaryTask) {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Minute)
	defer cancel()
	_ = s.st.MarkAIJobRunning(ctx, task.jobID)

	cfg := LoadSettings(ctx, s.st)
	items := make([]preparedSummary, 0, len(task.sessions))

	for _, sess := range task.sessions {
		if ctx.Err() != nil {
			return
		}
		prev, _, _ := s.st.GetAISummary(ctx, sess.ID)
		_ = s.st.MarkAISummaryRunning(ctx, sess.ID, prev.Attempts+1)

		// 派生会话优先复用父摘要，只采样分叉点之后的增量内容。
		lineageMode := "own"
		parentSummary := ""
		sample := ""
		inherited := false

		if sess.ParentID != "" {
			if pa, found, _ := s.st.GetAISummary(ctx, sess.ParentID); found && pa.Status == "ok" && pa.Summary != "" {
				parentSummary = pa.Summary
				if sres, err := scanner.SampleForSummaryFrom(sess.FilePath, sess.ForkPointUUID, cfg.MaxChars, cfg.IncludeConclusionTail); err == nil && sres.ForkFound {
					if sres.IncrementMessages < 3 {
						s.inheritSummary(ctx, task.jobID, prev, pa, sess)
						inherited = true
					} else if sres.Text != "" {
						sample = sres.Text
						lineageMode = "delta"
					}
				}
			}
		}
		if inherited {
			continue
		}
		if sample == "" {
			var err error
			sample, err = scanner.SampleForSummary(sess.FilePath, cfg.MaxChars, cfg.IncludeConclusionTail)
			if err != nil {
				s.failSummary(ctx, task.jobID, sess.ID, prev, "读取会话失败: "+err.Error(), 0)
				continue
			}
			if parentSummary != "" {
				lineageMode = "delta"
			}
		}

		items = append(items, preparedSummary{
			sess: sess, prev: prev, sample: sample, parent: parentSummary, lineageMode: lineageMode,
		})
	}

	if len(items) > 0 {
		if len(items) == 1 || cfg.SummaryBatchSize <= 1 {
			for _, item := range items {
				s.processOne(ctx, task.jobID, item)
			}
		} else {
			s.processModelBatch(ctx, task.jobID, items)
		}
	}

	_ = s.st.FinishAIJobIfComplete(ctx, task.jobID)
	s.broadcast("ai")
}

// processModelBatch 一次请求生成整批概要；失败或缺项时逐条重试。
func (s *Service) processModelBatch(ctx context.Context, jobID string, items []preparedSummary) {
	batch := make([]BatchSummaryItem, 0, len(items))
	for _, item := range items {
		project, _ := s.st.GetProject(ctx, item.sess.ProjectID)
		batch = append(batch, BatchSummaryItem{
			Session: item.sess, Project: project.Name,
			Sample: item.sample, ParentSummary: item.parent,
		})
	}

	res, err := s.gen.Generate(ctx, buildBatchSummaryRequest(batch))
	if err != nil {
		log.Printf("AI 批量概要失败 job=%s count=%d: %v，改为逐条重试", jobID, len(items), err)
		for _, item := range items {
			s.processOne(ctx, jobID, item)
		}
		return
	}
	payloads, err := parseBatchSummary(res)
	if err != nil {
		log.Printf("AI 批量概要解析失败 job=%s count=%d: %v，改为逐条重试", jobID, len(items), err)
		for _, item := range items {
			s.processOne(ctx, jobID, item)
		}
		return
	}
	s.setModel(res.Model)

	for _, item := range items {
		payload, ok := payloads[item.sess.ID]
		if !ok {
			log.Printf("批量概要缺少会话 %s，改为单条重试", item.sess.ID)
			s.processOne(ctx, jobID, item)
			continue
		}
		s.writeSummary(ctx, jobID, item, payload, res, 1, len(items))
	}
}

// processOne 处理单个会话，用于单条批次或批量失败后的重试。
func (s *Service) processOne(ctx context.Context, jobID string, item preparedSummary) {
	project, _ := s.st.GetProject(ctx, item.sess.ProjectID)
	req := buildSummaryRequest(item.sess, project.Name, item.sample, item.parent)

	var res Result
	var genErr error
	used := 0
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		used = attempt
		res, genErr = s.gen.Generate(ctx, req)
		if genErr == nil {
			break
		}
		kind := kindOf(genErr)
		if kind == KindAuth || kind == KindUnavailable || kind == KindSchema {
			break
		}
		if attempt < maxAttempts {
			select {
			case <-time.After(retryBackoff[attempt-1]):
			case <-s.ctx.Done():
				return
			}
		}
	}
	if genErr != nil {
		s.failSummary(ctx, jobID, item.sess.ID, item.prev, genErr.Error(), used)
		return
	}

	payload, err := parseSummary(res)
	if err != nil {
		s.failSummary(ctx, jobID, item.sess.ID, item.prev, err.Error(), used)
		return
	}
	s.setModel(res.Model)
	s.writeSummary(ctx, jobID, item, payload, res, used, 1)
}

// writeSummary 写入单条概要。share 用于把批量调用的用量分摊到每个会话。
func (s *Service) writeSummary(ctx context.Context, jobID string, item preparedSummary, payload summaryPayload, res Result, used, share int) {
	if used <= 0 {
		used = 1
	}
	if share <= 0 {
		share = 1
	}
	summary := model.AISummary{
		SessionID:      item.sess.ID,
		Summary:        payload.Summary,
		SuggestedTitle: payload.Title,
		Topics:         payload.Topics,
		Model:          res.Model,
		Profile:        string(res.Profile),
		Status:         "ok",
		Confidence:     payload.Confidence,
		LineageMode:    item.lineageMode,
		Drift:          payload.Drift,
		SourceMtime:    item.sess.FileMtime,
		SourceSize:     item.sess.FileSize,
		InTokens:       res.InTokens / share,
		OutTokens:      res.OutTokens / share,
		CostUSD:        res.CostUSD / float64(share),
		Attempts:       item.prev.Attempts + used,
		UpdatedAt:      time.Now().Unix(),
	}
	if err := s.st.UpsertAISummary(ctx, summary); err != nil {
		log.Printf("写入 AI 概要失败 %s: %v", item.sess.ID, err)
		_ = s.st.MarkAISummaryFailed(ctx, item.sess.ID, err.Error(), summary.Attempts)
		_ = s.st.IncAIJob(ctx, jobID, true)
		return
	}
	_ = s.st.IncAIJob(ctx, jobID, false)
	_ = s.st.UpdateFTSSummary(ctx, item.sess.ID)
}

// inheritSummary 对分叉后几乎没有新内容的会话直接复用父摘要，不调用模型。
func (s *Service) inheritSummary(ctx context.Context, jobID string, prev model.AISummary, parent model.AISummary, sess model.Session) {
	s.mu.RLock()
	profile := s.profile
	s.mu.RUnlock()

	summary := model.AISummary{
		SessionID:   sess.ID,
		Summary:     parent.Summary,
		Topics:      parent.Topics,
		Model:       "inherited",
		Profile:     string(profile),
		Status:      "ok",
		Confidence:  parent.Confidence,
		LineageMode: "inherited",
		Drift:       0,
		SourceMtime: sess.FileMtime,
		SourceSize:  sess.FileSize,
		Attempts:    prev.Attempts,
		UpdatedAt:   time.Now().Unix(),
	}
	if err := s.st.UpsertAISummary(ctx, summary); err != nil {
		_ = s.st.MarkAISummaryFailed(ctx, sess.ID, err.Error(), prev.Attempts)
		_ = s.st.IncAIJob(ctx, jobID, true)
		return
	}
	_ = s.st.IncAIJob(ctx, jobID, false)
	_ = s.st.UpdateFTSSummary(ctx, sess.ID)
}

// sortByLineage 按谱系深度升序排列，父会话优先；存在环时回退为深度 0。
func sortByLineage(sessions []model.Session) {
	parentOf := make(map[string]string, len(sessions))
	for _, sess := range sessions {
		parentOf[sess.ID] = sess.ParentID
	}
	depths := make(map[string]int, len(sessions))
	var depth func(id string, seen map[string]bool) int
	depth = func(id string, seen map[string]bool) int {
		if d, ok := depths[id]; ok {
			return d
		}
		if seen[id] {
			return 0
		}
		seen[id] = true
		parent := parentOf[id]
		if parent == "" {
			depths[id] = 0
			return 0
		}
		d := depth(parent, seen) + 1
		depths[id] = d
		return d
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		return depth(sessions[i].ID, map[string]bool{}) < depth(sessions[j].ID, map[string]bool{})
	})
}

func (s *Service) failSummary(ctx context.Context, jobID, sessionID string, prev model.AISummary, msg string, used int) {
	attempts := prev.Attempts + used
	if attempts == 0 {
		attempts = prev.Attempts + 1
	}
	_ = s.st.MarkAISummaryFailed(ctx, sessionID, msg, attempts)
	_ = s.st.IncAIJob(ctx, jobID, true)
	log.Printf("AI 概要失败 %s: %s", sessionID, msg)
}

// --- 项目级归类 ---

const (
	categorizeBatch   = 40
	maxFolderDepth    = 2
	minFolderSessions = 2
)

// Categorize 为项目内未归类的会话生成归类建议。默认只产出建议，不落库。
// 候选为空时返回原因，供前端展示。
func (s *Service) Categorize(ctx context.Context, projectID int64, reevaluate bool) (string, int, string, error) {
	if err := s.ensureProbed(ctx); err != nil {
		return "", 0, "", err
	}
	cfg := LoadSettings(ctx, s.st)
	if !cfg.Enabled {
		return "", 0, "", errors.New("AI 功能已关闭")
	}
	if id, ok, err := s.st.FindActiveJob(ctx, "categorize", projectID); err == nil && ok {
		return id, 0, "", nil
	}
	project, err := s.st.GetProject(ctx, projectID)
	if err != nil {
		return "", 0, "", errors.New("项目不存在")
	}
	if excluded(cfg, project.Cwd) {
		return "", 0, "", errors.New("该项目已加入 AI 排除名单")
	}

	sessions, err := s.st.CategorizeCandidates(ctx, projectID, reevaluate)
	if err != nil {
		return "", 0, "", err
	}
	jobID := newID()
	now := time.Now().Unix()
	if err := s.st.CreateAIJob(ctx, model.AIJob{
		ID: jobID, Kind: "categorize", ProjectID: projectID,
		Status: "queued", Total: len(sessions), CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		return "", 0, "", err
	}
	if len(sessions) == 0 {
		_ = s.st.FinishAIJobIfComplete(ctx, jobID)
		return jobID, 0, s.emptyCategorizeReason(ctx, projectID, reevaluate), nil
	}
	go s.processCategorize(jobID, projectID, sessions, cfg)
	s.broadcast("ai")
	return jobID, len(sessions), "", nil
}

func (s *Service) emptyCategorizeReason(ctx context.Context, projectID int64, reevaluate bool) string {
	total, _ := s.st.CountProjectSessions(ctx, projectID)
	if total == 0 {
		return "当前项目没有会话"
	}
	withSummary, _ := s.st.CountProjectSummaries(ctx, projectID)
	if withSummary == 0 {
		return "会话尚未生成概要，请稍后重试"
	}
	if !reevaluate {
		return "会话都已有归类，如需重新整理请选择「重新评估」"
	}
	return "没有可整理的会话"
}

func (s *Service) processCategorize(jobID string, projectID int64, sessions []model.Session, cfg model.AISettings) {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Minute)
	defer cancel()
	_ = s.st.MarkAIJobRunning(ctx, jobID)

	project, err := s.st.GetProject(ctx, projectID)
	if err != nil {
		_ = s.st.FailAIJob(ctx, jobID, "项目不存在")
		return
	}
	folders, err := s.st.ListFolders(ctx, projectID)
	if err != nil {
		_ = s.st.FailAIJob(ctx, jobID, err.Error())
		return
	}
	nodes, pathByID := folderNodes(folders)
	folderMap, _ := s.st.SessionFolderMap(ctx, projectID)

	ids := make([]string, 0, len(sessions))
	for _, sess := range sessions {
		ids = append(ids, sess.ID)
	}
	summaries, _ := s.st.ListAISummaries(ctx, ids)

	var allFolders []FolderSuggestionPayload
	var allAssignments []AssignmentPayload
	failed := 0
	lastErr := ""

	for start := 0; start < len(sessions); start += categorizeBatch {
		end := start + categorizeBatch
		if end > len(sessions) {
			end = len(sessions)
		}
		items := make([]CategorizeItem, 0, end-start)
		for _, sess := range sessions[start:end] {
			a := summaries[sess.ID]
			item := CategorizeItem{
				SessionID: sess.ID, Title: sess.Title, TitleSource: titleSource(sess),
				Summary: a.Summary, Topics: a.Topics, Drift: a.Drift,
			}
			if sess.ParentID != "" {
				if fid, ok := folderMap[sess.ParentID]; ok && fid != nil {
					item.ParentFolder = pathByID[*fid]
				}
			}
			items = append(items, item)
		}

		res, err := s.gen.Generate(ctx, buildCategorizeRequest(project.Name, nodes, items))
		if err != nil {
			log.Printf("AI 归类失败 job=%s project=%d batch=%d-%d: %v", jobID, projectID, start, end, err)
			lastErr = err.Error()
			_ = s.st.UpdateAIJob(ctx, jobID, start, failed+len(items), "running", err.Error())
			failed += len(items)
			continue
		}
		payload, err := parseCategorize(res)
		if err != nil {
			log.Printf("AI 归类结果解析失败 job=%s project=%d batch=%d-%d: %v", jobID, projectID, start, end, err)
			lastErr = err.Error()
			failed += len(items)
			continue
		}
		allFolders = append(allFolders, payload.Folders...)
		allAssignments = append(allAssignments, payload.Assignments...)
		_ = s.st.UpdateAIJob(ctx, jobID, end, failed, "running", "")
		s.broadcast("ai")
	}

	if len(allFolders) > 0 || len(allAssignments) > 0 {
		// 模型可能在 folders.name 与 assignments.new_folder 之间出现空格或大小写差异，
		// 先统一成 folders 中的规范名称，再做顶级限流和写库。
		canonical := make(map[string]string, len(allFolders))
		for _, f := range allFolders {
			if name := strings.TrimSpace(f.Name); name != "" {
				canonical[normalizeName(name)] = name
			}
		}
		for i := range allAssignments {
			nf := strings.TrimSpace(allAssignments[i].NewFolder)
			if nf == "" {
				continue
			}
			if name, ok := canonical[normalizeName(nf)]; ok {
				allAssignments[i].NewFolder = name
			} else {
				allAssignments[i].NewFolder = nf
			}
		}

		// 新建顶级归类最多 9 个，超出的合并到「其他」。
		allFolders, allAssignments = capTopFolders(allFolders, allAssignments, nodes)

		refToID := make(map[int]int64, len(nodes))
		for _, n := range nodes {
			refToID[n.Ref] = n.ID
		}

		var fs []store.FolderSuggestion
		for _, f := range allFolders {
			name := strings.TrimSpace(f.Name)
			if name == "" {
				continue
			}
			fs = append(fs, store.FolderSuggestion{
				JobID: jobID, ParentID: refToID[f.ParentRef],
				ParentName: strings.TrimSpace(f.ParentName),
				Name:       name, Description: f.Description, Confidence: f.Confidence,
			})
		}

		seen := make(map[string]bool, len(allAssignments))
		var ss []store.SessionSuggestion
		for _, a := range allAssignments {
			if a.SessionID == "" || seen[a.SessionID] {
				continue
			}
			seen[a.SessionID] = true
			folderID := refToID[a.Ref]
			action := "reuse"
			if folderID == 0 {
				if strings.TrimSpace(a.NewFolder) != "" {
					action = "new_folder"
				} else {
					action = "keep"
				}
			}
			ss = append(ss, store.SessionSuggestion{
				JobID: jobID, SessionID: a.SessionID, FolderID: folderID,
				NewFolder: strings.TrimSpace(a.NewFolder), Action: action,
				Confidence: a.Confidence, Reason: a.Reason,
			})
		}
		if err := s.st.CreateFolderSuggestions(ctx, fs); err != nil {
			log.Printf("写入归类建议失败: %v", err)
		}
		if err := s.st.CreateSessionSuggestions(ctx, ss); err != nil {
			log.Printf("写入会话归类建议失败: %v", err)
		}
	}

	status := "done"
	if failed >= len(sessions) && len(sessions) > 0 {
		status = "failed"
	}
	_ = s.st.UpdateAIJob(ctx, jobID, len(sessions), failed, status, lastErr)
	if cfg.AutoCategorize && status == "done" {
		if _, err := s.ApplySuggestions(ctx, jobID, nil, nil); err != nil {
			log.Printf("自动应用归类失败: %v", err)
		}
	}
	s.broadcast("ai")
}

// capTopFolders 限制新建的一级归类数量：保留会话数最多的 9 个，
// 其余归类及其子级合并到「其他」。已有归类不占名额。
func capTopFolders(folders []FolderSuggestionPayload, assignments []AssignmentPayload, existing []FolderNode) ([]FolderSuggestionPayload, []AssignmentPayload) {
	const maxTop = 9

	// 模型可能主动创建「其他」。未超额时这不是用户预期的行为，需要移除。
	var modelOther string
	for _, f := range folders {
		if f.ParentRef == 0 && strings.TrimSpace(f.ParentName) == "" && strings.TrimSpace(f.Name) == "其他" {
			modelOther = strings.TrimSpace(f.Name)
			break
		}
	}

	counts := make(map[string]int, len(folders))
	for _, a := range assignments {
		if a.NewFolder != "" {
			counts[a.NewFolder]++
		}
	}

	var tops []int
	for i, f := range folders {
		if f.ParentRef == 0 && strings.TrimSpace(f.ParentName) == "" {
			tops = append(tops, i)
		}
	}
	if len(tops) <= maxTop {
		if modelOther == "" {
			return folders, assignments
		}
		// 未超额：移除模型自建的「其他」，其会话保持未归类。
		kept := make([]FolderSuggestionPayload, 0, len(folders))
		for _, f := range folders {
			if strings.TrimSpace(f.Name) == modelOther && f.ParentRef == 0 && strings.TrimSpace(f.ParentName) == "" {
				continue
			}
			kept = append(kept, f)
		}
		for i := range assignments {
			if assignments[i].NewFolder == modelOther {
				assignments[i].NewFolder = ""
				assignments[i].Ref = 0
				reason := strings.TrimSpace(assignments[i].Reason)
				if reason != "" {
					reason += " "
				}
				assignments[i].Reason = reason + "（未超过 9 个顶级归类，不使用「其他」）"
			}
		}
		return kept, assignments
	}

	sort.SliceStable(tops, func(i, j int) bool {
		a, b := folders[tops[i]], folders[tops[j]]
		if counts[a.Name] != counts[b.Name] {
			return counts[a.Name] > counts[b.Name]
		}
		return a.Confidence > b.Confidence
	})

	dropped := make(map[string]bool, len(tops)-maxTop)
	for _, idx := range tops[maxTop:] {
		dropped[folders[idx].Name] = true
	}
	// 被合并的顶级，其二级子类一并合并。
	for _, f := range folders {
		if f.ParentName != "" && dropped[f.ParentName] {
			dropped[f.Name] = true
		}
	}

	kept := make([]FolderSuggestionPayload, 0, len(folders))
	hasOther := false
	for _, f := range folders {
		if dropped[f.Name] {
			continue
		}
		if strings.TrimSpace(f.Name) == "其他" {
			hasOther = true
		}
		kept = append(kept, f)
	}

	existingOtherRef := 0
	for _, n := range existing {
		if len(n.Path) == 1 && n.Path[0] == "其他" {
			existingOtherRef = n.Ref
			break
		}
	}
	if !hasOther && existingOtherRef == 0 {
		kept = append(kept, FolderSuggestionPayload{
			Name: "其他", Description: "顶级归类数量超过上限后合并的会话", Confidence: 0.5,
		})
	}

	for i := range assignments {
		name := assignments[i].NewFolder
		if name == "" || !dropped[name] {
			continue
		}
		reason := strings.TrimSpace(assignments[i].Reason)
		if reason != "" {
			reason += " "
		}
		assignments[i].Reason = reason + "（顶级归类超过 9 个，归入「其他」）"
		if existingOtherRef > 0 {
			assignments[i].Ref = existingOtherRef
			assignments[i].NewFolder = ""
		} else {
			assignments[i].NewFolder = "其他"
		}
	}
	return kept, assignments
}

// Suggestions 是预览弹窗需要的两类建议。
type Suggestions struct {
	Job         model.AIJob               `json:"job"`
	Folders     []store.FolderSuggestion  `json:"folders"`
	Assignments []store.SessionSuggestion `json:"assignments"`
}

func (s *Service) ListSuggestions(ctx context.Context, jobID string) (Suggestions, error) {
	job, err := s.st.GetAIJob(ctx, jobID)
	if err != nil {
		return Suggestions{}, errors.New("任务不存在")
	}
	folders, err := s.st.ListFolderSuggestions(ctx, jobID)
	if err != nil {
		return Suggestions{}, err
	}
	assignments, err := s.st.ListSessionSuggestions(ctx, jobID)
	if err != nil {
		return Suggestions{}, err
	}
	if folders == nil {
		folders = []store.FolderSuggestion{}
	}
	if assignments == nil {
		assignments = []store.SessionSuggestion{}
	}

	// 回填展示用路径：已有归类查路径，新建归类按父子关系拼装。
	existing, _ := s.st.ListFolders(ctx, job.ProjectID)
	_, pathByID := buildPathIndex(existing)
	newPaths := resolveNewFolderPaths(folders, pathByID)
	normPaths := make(map[string][]string, len(newPaths))
	for name, p := range newPaths {
		normPaths[normalizeName(name)] = p
	}

	for i := range folders {
		if p := newPaths[folders[i].Name]; len(p) > 0 {
			folders[i].ParentPath = append([]string{}, p[:len(p)-1]...)
		}
	}
	for i := range assignments {
		switch {
		case assignments[i].FolderID > 0:
			assignments[i].FolderPath = append([]string{}, pathByID[assignments[i].FolderID]...)
		case assignments[i].NewFolder != "":
			p := newPaths[assignments[i].NewFolder]
			if len(p) == 0 {
				p = normPaths[normalizeName(assignments[i].NewFolder)]
			}
			assignments[i].FolderPath = append([]string{}, p...)
		}
	}

	return Suggestions{Job: job, Folders: folders, Assignments: assignments}, nil
}

// resolveNewFolderPaths 按父级引用拼出本批新建归类的完整路径。
func resolveNewFolderPaths(folders []store.FolderSuggestion, existingPaths map[int64][]string) map[string][]string {
	out := make(map[string][]string, len(folders))
	remaining := append([]store.FolderSuggestion{}, folders...)
	for len(remaining) > 0 {
		var next []store.FolderSuggestion
		progress := false
		for _, f := range remaining {
			var parent []string
			switch {
			case f.ParentID > 0:
				parent = existingPaths[f.ParentID]
			case f.ParentName != "":
				p, ok := out[f.ParentName]
				if !ok {
					next = append(next, f)
					continue
				}
				parent = p
			}
			p := append(append([]string{}, parent...), f.Name)
			out[f.Name] = p
			progress = true
		}
		if !progress {
			break
		}
		remaining = next
	}
	return out
}

// ApplySuggestions 落地归类建议。folderIDs / sessionIDs 为空表示全部。
func (s *Service) ApplySuggestions(ctx context.Context, jobID string, folderIDs, sessionIDs []int64) (int, error) {
	job, err := s.st.GetAIJob(ctx, jobID)
	if err != nil {
		return 0, errors.New("任务不存在")
	}
	folders, err := s.st.ListFolderSuggestions(ctx, jobID)
	if err != nil {
		return 0, err
	}
	assignments, err := s.st.ListSessionSuggestions(ctx, jobID)
	if err != nil {
		return 0, err
	}

	existing, err := s.st.ListFolders(ctx, job.ProjectID)
	if err != nil {
		return 0, err
	}
	index, pathByID := buildPathIndex(existing)
	validIDs := make(map[int64]bool, len(existing))
	for _, f := range existing {
		validIDs[f.ID] = true
	}

	wantFolder := toSet(folderIDs)
	wantSession := toSet(sessionIDs)

	// 统计本批新建归类实际会绑定多少会话，用于「至少 2 个」门槛。
	// 父级自身可能没有会话，但子级有，所以父级按「自身 + 子级」计算。
	counts := make(map[string]int)
	for _, a := range assignments {
		if a.Applied {
			continue
		}
		if len(wantSession) > 0 && !wantSession[a.ID] {
			continue
		}
		if a.NewFolder != "" {
			counts[normalizeName(a.NewFolder)]++
		}
	}
	parentOf := make(map[string]string)
	for _, f := range folders {
		if f.ParentName != "" {
			parentOf[normalizeName(f.Name)] = normalizeName(f.ParentName)
		}
	}
	effective := make(map[string]int, len(counts))
	for name, n := range counts {
		effective[name] = n
	}
	for child, parent := range parentOf {
		effective[parent] += counts[child]
	}

	nameToID := make(map[string]int64)
	newDepth := make(map[int64]int)
	now := time.Now().Unix()
	var appliedFolderIDs []int64

	for _, fsg := range folders {
		if fsg.Applied {
			continue
		}
		if len(wantFolder) > 0 && !wantFolder[fsg.ID] {
			continue
		}

		var parentID *int64
		parentDepth := 0
		if fsg.ParentID > 0 && validIDs[fsg.ParentID] {
			v := fsg.ParentID
			parentID = &v
			parentDepth = len(pathByID[v])
		} else if fsg.ParentName != "" {
			if id, ok := nameToID[normalizeName(fsg.ParentName)]; ok {
				v := id
				parentID = &v
				parentDepth = newDepth[v]
			}
		}

		// 父级已经是第 2 层时降级为复用父级，不产生第 3 层。
		if parentID != nil && parentDepth >= maxFolderDepth {
			nameToID[normalizeName(fsg.Name)] = *parentID
			appliedFolderIDs = append(appliedFolderIDs, fsg.ID)
			continue
		}
		if effective[normalizeName(fsg.Name)] < minFolderSessions {
			continue
		}

		id, err := s.st.CreateAIFolder(ctx, job.ProjectID, fsg.Name, parentID, fsg.Description, now)
		if err != nil {
			return 0, err
		}
		nameToID[normalizeName(fsg.Name)] = id
		newDepth[id] = parentDepth + 1
		appliedFolderIDs = append(appliedFolderIDs, fsg.ID)
	}

	applied := 0
	var appliedSessionIDs []int64
	for _, a := range assignments {
		if a.Applied {
			continue
		}
		if len(wantSession) > 0 && !wantSession[a.ID] {
			continue
		}

		var target int64
		ok := false
		switch {
		case a.FolderID > 0 && validIDs[a.FolderID]:
			target, ok = a.FolderID, true
		case a.NewFolder != "":
			target, ok = nameToID[normalizeName(a.NewFolder)]
		case len(a.FolderPath) > 0:
			// 兼容升级前生成、尚未应用的建议。
			target, ok = lookupPath(index, a.FolderPath)
			if !ok && len(a.FolderPath) > 1 {
				target, ok = lookupPath(index, a.FolderPath[:len(a.FolderPath)-1])
			}
		}
		if !ok {
			continue
		}
		if err := s.st.SetSessionFolderAI(ctx, a.SessionID, target); err != nil {
			continue
		}
		applied++
		appliedSessionIDs = append(appliedSessionIDs, a.ID)
	}

	_ = s.st.MarkFolderSuggestionsApplied(ctx, appliedFolderIDs)
	_ = s.st.MarkSessionSuggestionsApplied(ctx, appliedSessionIDs)
	s.broadcast("scan")
	return applied, nil
}

// DiscardSuggestions 丢弃一次整理的结果。
func (s *Service) DiscardSuggestions(ctx context.Context, jobID string) error {
	if err := s.st.DeleteAISuggestions(ctx, jobID); err != nil {
		return err
	}
	_ = s.st.UpdateAIJob(ctx, jobID, 0, 0, "canceled", "")
	s.broadcast("ai")
	return nil
}

func folderNodes(folders []model.Folder) ([]FolderNode, map[int64][]string) {
	byID := make(map[int64]model.Folder, len(folders))
	for _, f := range folders {
		byID[f.ID] = f
	}
	pathByID := make(map[int64][]string, len(folders))
	var resolve func(id int64) []string
	resolve = func(id int64) []string {
		f, ok := byID[id]
		if !ok {
			return nil
		}
		if f.ParentID == nil {
			return []string{f.Name}
		}
		parent := resolve(*f.ParentID)
		if parent == nil {
			return []string{f.Name}
		}
		return append(append([]string{}, parent...), f.Name)
	}
	nodes := make([]FolderNode, 0, len(folders))
	for i, f := range folders {
		p := resolve(f.ID)
		pathByID[f.ID] = p
		source := f.Source
		if source == "" {
			source = "user"
		}
		nodes = append(nodes, FolderNode{
			ID: f.ID, Ref: i + 1, Path: p, Source: source, Description: f.Description,
		})
	}
	return nodes, pathByID
}

func buildPathIndex(folders []model.Folder) (map[string]int64, map[int64][]string) {
	byID := make(map[int64]model.Folder, len(folders))
	for _, f := range folders {
		byID[f.ID] = f
	}
	var resolve func(id int64) []string
	resolve = func(id int64) []string {
		f, ok := byID[id]
		if !ok {
			return nil
		}
		if f.ParentID == nil {
			return []string{f.Name}
		}
		parent := resolve(*f.ParentID)
		return append(append([]string{}, parent...), f.Name)
	}
	index := make(map[string]int64, len(folders))
	pathByID := make(map[int64][]string, len(folders))
	for _, f := range folders {
		p := resolve(f.ID)
		pathByID[f.ID] = p
		index[pathKey(p)] = f.ID
	}
	return index, pathByID
}

func lookupPath(index map[string]int64, path []string) (int64, bool) {
	id, ok := index[pathKey(path)]
	return id, ok
}

func pathKey(path []string) string {
	parts := make([]string, 0, len(path))
	for _, p := range path {
		parts = append(parts, normalizeName(p))
	}
	return strings.Join(parts, "/")
}

func normalizeName(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		switch r {
		case ' ', '\t', '　', '/', '\\', '-', '_':
			continue
		}
		if r >= 0xFF01 && r <= 0xFF5E {
			r -= 0xFEE0
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func toSet(ids []int64) map[int64]bool {
	if len(ids) == 0 {
		return nil
	}
	out := make(map[int64]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// Status 汇总当前能力、队列与用量。
func (s *Service) Status(ctx context.Context, projectID int64) Status {
	cfg := LoadSettings(ctx, s.st)
	out := Status{Enabled: cfg.Enabled, Queue: len(s.queue)}
	s.mu.RLock()
	out.Available = s.available
	out.Probed = s.probed
	out.Profile = string(s.profile)
	out.Version = s.version
	out.Model = s.model
	out.Error = s.probeErr
	s.mu.RUnlock()

	if projectID > 0 {
		out.Pending, _ = s.st.CountPendingSummaries(ctx, projectID, cfg.MinMessages)
		if job, err := s.st.LatestAIJob(ctx, "summary", projectID); err == nil {
			out.Job = &job
		}
	}
	out.SummaryCount, out.Failed, out.InTokens, out.OutTokens, out.CostUSD, _ =
		s.st.AITotals(ctx)
	return out
}

type Status struct {
	Available    bool         `json:"available"`
	Probed       bool         `json:"probed"`
	Profile      string       `json:"profile,omitempty"`
	Version      string       `json:"version,omitempty"`
	Model        string       `json:"model,omitempty"`
	Error        string       `json:"error,omitempty"`
	Enabled      bool         `json:"enabled"`
	Pending      int          `json:"pending"`
	Queue        int          `json:"queue"`
	Job          *model.AIJob `json:"job,omitempty"`
	SummaryCount int          `json:"summary_count"`
	Failed       int          `json:"failed"`
	InTokens     int          `json:"in_tokens"`
	OutTokens    int          `json:"out_tokens"`
	CostUSD      float64      `json:"cost_usd"`
}

// ParseSettingsPatch 解析 PATCH /api/ai/settings 的部分更新。
func ParseSettingsPatch(base model.AISettings, raw []byte) (model.AISettings, error) {
	if len(raw) == 0 {
		return base, nil
	}
	var patch struct {
		Enabled               *bool     `json:"enabled"`
		Model                 *string   `json:"model"`
		Profile               *string   `json:"profile"`
		MinMessages           *int      `json:"min_messages"`
		MaxChars              *int      `json:"max_chars"`
		IncludeConclusionTail *bool     `json:"include_conclusion_tail"`
		AutoEnqueueLimit      *int      `json:"auto_enqueue_limit"`
		SummaryBatchSize      *int      `json:"summary_batch_size"`
		AutoCategorize        *bool     `json:"auto_categorize"`
		ExcludeProjects       *[]string `json:"exclude_projects"`
	}
	if err := json.Unmarshal(raw, &patch); err != nil {
		return base, fmt.Errorf("参数格式错误: %w", err)
	}
	if patch.Enabled != nil {
		base.Enabled = *patch.Enabled
	}
	if patch.Model != nil {
		base.Model = *patch.Model
	}
	if patch.Profile != nil {
		base.Profile = *patch.Profile
	}
	if patch.MinMessages != nil {
		base.MinMessages = *patch.MinMessages
	}
	if patch.MaxChars != nil {
		base.MaxChars = *patch.MaxChars
	}
	if patch.IncludeConclusionTail != nil {
		base.IncludeConclusionTail = *patch.IncludeConclusionTail
	}
	if patch.AutoEnqueueLimit != nil {
		base.AutoEnqueueLimit = *patch.AutoEnqueueLimit
	}
	if patch.SummaryBatchSize != nil {
		base.SummaryBatchSize = *patch.SummaryBatchSize
	}
	if patch.AutoCategorize != nil {
		base.AutoCategorize = *patch.AutoCategorize
	}
	if patch.ExcludeProjects != nil {
		base.ExcludeProjects = *patch.ExcludeProjects
	}
	return base, nil
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
