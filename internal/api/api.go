// Author: zhangweiwei
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"claude-session/internal/creator"
	"claude-session/internal/model"
	"claude-session/internal/opener"
	"claude-session/internal/scanner"
	"claude-session/internal/store"
	"claude-session/internal/trash"
)

type Server struct {
	store    *store.Store
	scanner  *scanner.Scanner
	opener   *opener.Opener
	creator  *creator.Creator
	trash    *trash.Manager
	web      fs.FS
	syncMu   sync.Mutex
	lastScan time.Time
	lastStat scanner.Result

	subMu sync.Mutex
	subs  map[chan string]struct{}

	hookMu   sync.Mutex
	lastHook time.Time
}

func New(st *store.Store, sc *scanner.Scanner, op *opener.Opener, cr *creator.Creator, tr *trash.Manager, web fs.FS) *Server {
	return &Server{
		store: st, scanner: sc, opener: op, creator: cr, trash: tr, web: web,
		subs: make(map[chan string]struct{}),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/projects", s.listProjects)
	mux.HandleFunc("POST /api/projects", s.createProject)
	mux.HandleFunc("PATCH /api/projects/{id}", s.patchProject)
	mux.HandleFunc("POST /api/projects/{id}/touch", s.touchProject)
	mux.HandleFunc("POST /api/projects/{id}/move", s.movePinnedProject)
	mux.HandleFunc("DELETE /api/projects/{id}", s.deleteProject)
	mux.HandleFunc("POST /api/projects/{id}/trash/purge", s.purgeTrash)
	mux.HandleFunc("GET /api/projects/{id}/folders", s.listFolders)
	mux.HandleFunc("POST /api/projects/{id}/folders", s.createFolder)
	mux.HandleFunc("PATCH /api/projects/{id}/folders/{folder}", s.renameFolder)
	mux.HandleFunc("POST /api/projects/{id}/folders/{folder}/move", s.moveFolder)
	mux.HandleFunc("DELETE /api/projects/{id}/folders/{folder}", s.deleteFolder)
	mux.HandleFunc("GET /api/sessions", s.listSessions)
	mux.HandleFunc("POST /api/sessions", s.createSession)
	mux.HandleFunc("POST /api/sessions/batch-delete", s.batchDelete)
	mux.HandleFunc("GET /api/sessions/{id}", s.getSession)
	mux.HandleFunc("GET /api/sessions/{id}/ancestors", s.sessionAncestors)
	mux.HandleFunc("GET /api/sessions/{id}/export", s.exportSession)
	mux.HandleFunc("PATCH /api/sessions/{id}", s.patchSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.deleteSession)
	mux.HandleFunc("POST /api/sessions/{id}/open", s.openSession)
	mux.HandleFunc("POST /api/sessions/{id}/branch", s.branchSession)
	mux.HandleFunc("POST /api/sessions/{id}/fork", s.forkSession)
	mux.HandleFunc("POST /api/sessions/{id}/compact", s.compactSession)
	mux.HandleFunc("POST /api/sessions/{id}/restore", s.restoreSession)
	mux.HandleFunc("POST /api/sync", s.sync)
	mux.HandleFunc("GET /api/fs/dirs", s.listDirs)
	mux.HandleFunc("POST /api/hooks/refresh", s.hookRefresh)
	mux.HandleFunc("GET /api/hooks/config", s.hooksConfig)
	mux.HandleFunc("GET /api/events", s.events)
	mux.Handle("GET /", noCache(http.FileServer(http.FS(s.web))))
	return s.withCommon(mux)
}

func (s *Server) withCommon(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOrigin(r) {
			writeErr(w, http.StatusForbidden, "只允许本页面发起的请求")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.ListProjects(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	total := 0
	for _, p := range projects {
		total += p.SessionCount
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"projects":  len(projects),
		"sessions":  total,
		"last_scan": s.lastScan,
		"opener": map[string]any{
			"available": s.opener.Available(),
		},
	})
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.ListProjects(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	raw := strings.TrimSpace(strings.Trim(body.Path, `"'`))
	if raw == "" {
		writeErr(w, http.StatusBadRequest, "项目路径不能为空")
		return
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "项目路径不合法")
		return
	}
	abs = filepath.Clean(abs)
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		writeErr(w, http.StatusBadRequest, "目录不存在或不可访问")
		return
	}

	projects, err := s.store.ListProjects(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, p := range projects {
		if strings.EqualFold(p.Cwd, abs) {
			writeJSON(w, http.StatusOK, p)
			return
		}
	}

	name := filepath.Base(strings.TrimRight(abs, `\/`))
	if name == "." || name == "" {
		name = abs
	}
	now := time.Now().Unix()
	id, err := s.store.UpsertProject(r.Context(), abs, scanner.EncodeCwd(abs), name, now)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, model.Project{
		ID: id, Cwd: abs, EncodedDir: scanner.EncodeCwd(abs), Name: name, LastScanAt: now,
	})
}

func (s *Server) patchProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "项目 ID 不合法")
		return
	}
	var body struct {
		Pinned *bool   `json:"pinned"`
		Alias  *string `json:"alias"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	if body.Pinned == nil && body.Alias == nil {
		writeErr(w, http.StatusBadRequest, "没有可更新的字段")
		return
	}
	if _, err := s.store.GetProject(r.Context(), projectID); err != nil {
		writeErr(w, http.StatusNotFound, "项目不存在")
		return
	}
	if body.Pinned != nil {
		if err := s.store.SetProjectPinned(r.Context(), projectID, *body.Pinned); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if body.Alias != nil {
		alias := strings.TrimSpace(*body.Alias)
		if len([]rune(alias)) > 40 {
			writeErr(w, http.StatusBadRequest, "别名不能超过 40 个字符")
			return
		}
		if err := s.store.SetProjectAlias(r.Context(), projectID, alias); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	project, err := s.store.GetProject(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (s *Server) touchProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "项目 ID 不合法")
		return
	}
	if _, err := s.store.GetProject(r.Context(), projectID); err != nil {
		writeErr(w, http.StatusNotFound, "项目不存在")
		return
	}
	if err := s.store.TouchProject(r.Context(), projectID, time.Now().Unix()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"touched": true})
}

func (s *Server) movePinnedProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "项目 ID 不合法")
		return
	}
	direction, ok := moveDirection(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "direction 只能是 up 或 down")
		return
	}
	project, err := s.store.GetProject(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "项目不存在")
		return
	}
	if !project.Pinned {
		writeErr(w, http.StatusBadRequest, "只有收藏的项目可以调整顺序")
		return
	}
	if err := s.store.MovePinnedProject(r.Context(), projectID, direction); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"moved": true})
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "项目 ID 不合法")
		return
	}
	if _, err := s.store.GetProject(r.Context(), projectID); err != nil {
		writeErr(w, http.StatusNotFound, "项目不存在")
		return
	}
	if err := s.store.ClearDisplayProject(r.Context(), projectID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	sessions, err := s.store.ListProjectSessions(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	// 先检查活跃会话，避免移动一半文件后才发现冲突。
	for _, sess := range sessions {
		if sess.State == "ok" && opener.SessionBusy(s.opener.SessionsDir(), sess.ID) {
			writeErr(w, http.StatusConflict, "项目下有正在运行的会话，请先在 Claude Code 中结束")
			return
		}
	}

	moved := make([]model.Session, 0)
	rollback := func() {
		for i := len(moved) - 1; i >= 0; i-- {
			if err := s.trash.Restore(moved[i]); err != nil {
				log.Printf("回滚会话 %s 失败: %v", moved[i].ID, err)
			}
		}
	}

	for _, sess := range sessions {
		if sess.State != "ok" {
			continue
		}
		if err := s.trash.Move(sess); err != nil {
			rollback()
			writeErr(w, http.StatusInternalServerError, "移动会话文件失败，已回滚: "+err.Error())
			return
		}
		moved = append(moved, sess)
	}

	if err := s.store.DeleteProject(r.Context(), projectID); err != nil {
		rollback()
		writeErr(w, http.StatusInternalServerError, "删除项目失败，已回滚: "+err.Error())
		return
	}

	s.broadcast("scan")
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "sessions": len(moved)})
}

func (s *Server) listFolders(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "项目 ID 不合法")
		return
	}
	folders, err := s.store.ListFolders(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"folders": folders})
}

func (s *Server) createFolder(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "项目 ID 不合法")
		return
	}
	var body struct {
		Name     string `json:"name"`
		ParentID *int64 `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeErr(w, http.StatusBadRequest, "文件夹名称不能为空")
		return
	}
	if body.ParentID != nil {
		ok, err := s.store.FolderExists(r.Context(), projectID, *body.ParentID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			writeErr(w, http.StatusBadRequest, "父归类不存在")
			return
		}
	}
	folder, err := s.store.CreateFolder(r.Context(), projectID, body.Name, body.ParentID, time.Now().Unix())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, folder)
}

func (s *Server) renameFolder(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "项目 ID 不合法")
		return
	}
	folderID, ok := pathID(r, "folder")
	if !ok {
		writeErr(w, http.StatusBadRequest, "归类 ID 不合法")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "归类名称不能为空")
		return
	}
	exists, err := s.store.FolderExists(r.Context(), projectID, folderID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		writeErr(w, http.StatusNotFound, "归类不存在")
		return
	}
	if err := s.store.RenameFolder(r.Context(), projectID, folderID, name); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"renamed": true, "name": name})
}

func (s *Server) moveFolder(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "项目 ID 不合法")
		return
	}
	folderID, ok := pathID(r, "folder")
	if !ok {
		writeErr(w, http.StatusBadRequest, "归类 ID 不合法")
		return
	}
	direction, ok := moveDirection(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "direction 只能是 up 或 down")
		return
	}
	exists, err := s.store.FolderExists(r.Context(), projectID, folderID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		writeErr(w, http.StatusNotFound, "归类不存在")
		return
	}
	if err := s.store.MoveFolder(r.Context(), projectID, folderID, direction); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"moved": true})
}

// moveDirection 解析排序请求的 direction 字段。
func moveDirection(r *http.Request) (string, bool) {
	var body struct {
		Direction string `json:"direction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return "", false
	}
	if body.Direction != "up" && body.Direction != "down" {
		return "", false
	}
	return body.Direction, true
}

func (s *Server) deleteFolder(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "项目 ID 不合法")
		return
	}
	folderID, ok := pathID(r, "folder")
	if !ok {
		writeErr(w, http.StatusBadRequest, "归类 ID 不合法")
		return
	}
	exists, err := s.store.FolderExists(r.Context(), projectID, folderID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		writeErr(w, http.StatusNotFound, "归类不存在")
		return
	}
	if err := s.store.DeleteFolder(r.Context(), projectID, folderID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcast("scan")
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathQueryID(r, "project_id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "缺少或非法的 project_id")
		return
	}
	filter := model.SessionFilter{
		ProjectID: projectID,
		Deleted:   r.URL.Query().Get("deleted") == "1",
	}
	switch raw := r.URL.Query().Get("folder_id"); raw {
	case "", "all":
	case "unfiled", "0":
		filter.Unfiled = true
	case "categorized":
		filter.Categorized = true
	default:
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "folder_id 不合法")
			return
		}
		filter.FolderID = &id
	}
	filter.Query = r.URL.Query().Get("q")
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			filter.Limit = n
		}
	}
	sessions, err := s.store.ListSessions(r.Context(), filter)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	active := opener.ActiveSessions(s.opener.SessionsDir())
	for i := range sessions {
		sessions[i].Active = active[sessions[i].ID]
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, err := s.store.GetSession(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "会话不存在")
		return
	}
	if sess.State == "missing" {
		writeErr(w, http.StatusNotFound, "会话文件已不存在，可能已被 Claude Code 清理")
		return
	}
	sess.Active = opener.SessionBusy(s.opener.SessionsDir(), sess.ID)
	path := sess.FilePath
	if sess.State == "deleted" {
		path = s.trash.Path(sess.ID)
	}
	messages, truncated, err := scanner.ReadMessages(path)
	if err != nil {
		if sess.State != "deleted" {
			writeErr(w, http.StatusInternalServerError, "读取会话文件失败: "+err.Error())
			return
		}
		messages, truncated = nil, false
	}
	writeJSON(w, http.StatusOK, model.SessionDetail{
		Session:   sess,
		Messages:  messages,
		Truncated: truncated,
	})
}

// sessionAncestors 返回从根到父会话的链路，用于详情页血缘导航。
func (s *Server) sessionAncestors(w http.ResponseWriter, r *http.Request) {
	lineage, err := s.store.Lineage(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(lineage))
	for _, item := range lineage {
		out = append(out, map[string]any{
			"id":    item.ID,
			"title": item.Title,
			"state": item.State,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ancestors": out})
}

func (s *Server) patchSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		FolderID   *int64  `json:"folder_id"`
		Title      *string `json:"title"`
		ProjectID  *int64  `json:"project_id"`
		FollowAuto bool    `json:"follow_auto"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	sess, err := s.store.GetSession(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "会话不存在")
		return
	}

	if body.FollowAuto {
		if err := s.store.SetSessionFollowAuto(r.Context(), id); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := s.store.InvalidateSession(r.Context(), id); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if _, err := s.scanner.Scan(r.Context()); err != nil {
			writeErr(w, http.StatusInternalServerError, "重新解析会话失败: "+err.Error())
			return
		}
		updated, err := s.store.GetSession(r.Context(), id)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		updated.Active = opener.SessionBusy(s.opener.SessionsDir(), id)
		writeJSON(w, http.StatusOK, updated)
		return
	}

	if body.Title != nil {
		title := strings.TrimSpace(*body.Title)
		if title == "" {
			writeErr(w, http.StatusBadRequest, "标题不能为空")
			return
		}
		if opener.SessionBusy(s.opener.SessionsDir(), id) {
			writeErr(w, http.StatusConflict, opener.ErrBusy.Error())
			return
		}
		if err := scanner.AppendCustomTitle(sess.FilePath, id, title); err != nil {
			writeErr(w, http.StatusInternalServerError, "写入会话标题失败: "+err.Error())
			return
		}
		if err := s.store.SetSessionTitle(r.Context(), id, title); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		sess.Title = title
		sess.CustomTitle = title
	}

	effectiveProject := sess.ProjectID
	if sess.DisplayProjectID != nil {
		effectiveProject = *sess.DisplayProjectID
	}

	if body.ProjectID != nil {
		target := *body.ProjectID
		if _, err := s.store.GetProject(r.Context(), target); err != nil {
			writeErr(w, http.StatusBadRequest, "目标项目不存在")
			return
		}
		if target != effectiveProject {
			var display *int64
			if target != sess.ProjectID {
				display = &target
			}
			if err := s.store.SetSessionProject(r.Context(), id, display); err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			sess.DisplayProjectID = display
			sess.FolderID = nil
			effectiveProject = target
		}
	}

	if body.FolderID != nil {
		var folderID *int64
		if *body.FolderID != 0 {
			ok, err := s.store.FolderExists(r.Context(), effectiveProject, *body.FolderID)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			if !ok {
				writeErr(w, http.StatusBadRequest, "归类不存在")
				return
			}
			folderID = body.FolderID
		}
		if err := s.store.SetSessionFolder(r.Context(), id, folderID); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		sess.FolderID = folderID
	}

	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProjectID    int64  `json:"project_id"`
		FolderID     int64  `json:"folder_id"`
		Title        string `json:"title"`
		FirstMessage string `json:"first_message"`
		Mode         string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	body.Title = strings.TrimSpace(body.Title)
	body.FirstMessage = strings.TrimSpace(body.FirstMessage)
	if body.FirstMessage == "" {
		writeErr(w, http.StatusBadRequest, "首条消息不能为空")
		return
	}
	project, err := s.store.GetProject(r.Context(), body.ProjectID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "项目不存在")
		return
	}
	if info, err := os.Stat(project.Cwd); err != nil || !info.IsDir() {
		writeErr(w, http.StatusUnprocessableEntity, "项目目录不存在")
		return
	}
	if body.FolderID != 0 {
		ok, err := s.store.FolderExists(r.Context(), project.ID, body.FolderID)
		if err != nil || !ok {
			writeErr(w, http.StatusBadRequest, "归类不存在")
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 150*time.Second)
	defer cancel()

	var id string
	switch body.Mode {
	case "draft":
		id, err = s.creator.CreateDraft(project, body.FirstMessage, body.Title)
	case "", "cli":
		id, err = s.creator.CreateCLI(ctx, project, body.FirstMessage, body.Title)
	default:
		writeErr(w, http.StatusBadRequest, "不支持的创建方式")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	scanCtx, scanCancel := context.WithTimeout(context.Background(), 60*time.Second)
	res, scanErr := s.scanner.Scan(scanCtx)
	scanCancel()
	if scanErr == nil {
		s.SetScanState(time.Now(), res)
	}
	if body.FolderID != 0 {
		if err := s.store.SetSessionFolder(r.Context(), id, &body.FolderID); err != nil {
			log.Printf("设置新会话归类失败: %v", err)
		}
	}
	s.broadcast("scan")

	sess, err := s.store.GetSession(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusCreated, map[string]any{
			"id": id, "warning": "会话已创建，但索引尚未完成，请稍后刷新",
		})
		return
	}
	writeJSON(w, http.StatusCreated, sess)
}

// branchSession 对应 Claude Code 的 /branch：复制历史到新会话，原会话保留。
func (s *Server) branchSession(w http.ResponseWriter, r *http.Request) {
	s.copySessionToNew(w, r, "branch")
}

// forkSession 对应 Claude Code 的 /fork：复制成独立会话，原会话保留。
func (s *Server) forkSession(w http.ResponseWriter, r *http.Request) {
	s.copySessionToNew(w, r, "fork")
}

// copyTitle 生成新会话标题：指定名称时直接使用该名称，
// 留空时按同项目内已有的「原标题-kind-数字」自动递增（001、002…）。
func (s *Server) copyTitle(ctx context.Context, sess model.Session, kind, name string) string {
	if name != "" {
		return name
	}
	base := sess.Title + "-" + kind
	prefix := base + "-"
	re := regexp.MustCompile("^" + regexp.QuoteMeta(prefix) + `(\d+)$`)
	max := 0
	if sessions, err := s.store.ListProjectSessions(ctx, sess.ProjectID); err == nil {
		for _, item := range sessions {
			if m := re.FindStringSubmatch(item.Title); m != nil {
				if n, err := strconv.Atoi(m[1]); err == nil && n > max {
					max = n
				}
			}
		}
	}
	return fmt.Sprintf("%s%03d", prefix, max+1)
}

func (s *Server) copySessionToNew(w http.ResponseWriter, r *http.Request, kind string) {
	var body struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	id := r.PathValue("id")
	sess, err := s.store.GetSession(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "会话不存在")
		return
	}
	if sess.State == "missing" {
		writeErr(w, http.StatusNotFound, "会话文件已不存在，可能已被 Claude Code 清理")
		return
	}
	if _, err := os.Stat(sess.FilePath); err != nil {
		writeErr(w, http.StatusNotFound, "会话文件已不存在")
		return
	}

	newID, err := s.creator.Fork(sess.FilePath, sess.ID, s.copyTitle(r.Context(), sess, kind, strings.TrimSpace(body.Name)))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	scanCtx, scanCancel := context.WithTimeout(context.Background(), 60*time.Second)
	res, scanErr := s.scanner.Scan(scanCtx)
	scanCancel()
	if scanErr == nil {
		s.SetScanState(time.Now(), res)
	}
	// 归类继承由扫描处理：解析 forkedFrom 后把父会话的归类复制到新会话。
	s.broadcast("scan")

	newSess, err := s.store.GetSession(r.Context(), newID)
	if err != nil {
		writeJSON(w, http.StatusCreated, map[string]any{
			"id": newID, "warning": "会话已复制，但索引尚未完成，请稍后刷新",
		})
		return
	}
	writeJSON(w, http.StatusCreated, newSess)
}

// compactSession 调用 Claude Code 的 /compact 压缩指定会话上下文。
func (s *Server) compactSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, err := s.store.GetSession(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "会话不存在")
		return
	}
	if sess.State == "missing" {
		writeErr(w, http.StatusNotFound, "会话文件已不存在，可能已被 Claude Code 清理")
		return
	}
	if _, err := os.Stat(sess.FilePath); err != nil {
		writeErr(w, http.StatusNotFound, "会话文件已不存在")
		return
	}
	if !s.creator.Available() {
		writeErr(w, http.StatusServiceUnavailable, "未找到 claude 可执行文件，无法压缩")
		return
	}
	if sess.Cwd == "" {
		writeErr(w, http.StatusUnprocessableEntity, "会话缺少工作目录，无法压缩")
		return
	}
	if info, err := os.Stat(sess.Cwd); err != nil || !info.IsDir() {
		writeErr(w, http.StatusUnprocessableEntity, "会话工作目录不存在，无法压缩")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 240*time.Second)
	defer cancel()
	if err := s.creator.Compact(ctx, sess.Cwd, sess.ID); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	scanCtx, scanCancel := context.WithTimeout(context.Background(), 60*time.Second)
	res, scanErr := s.scanner.Scan(scanCtx)
	scanCancel()
	if scanErr == nil {
		s.SetScanState(time.Now(), res)
	}
	s.broadcast("scan")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

// purgeTrash 清空当前项目的回收站：永久删除其中的会话文件与索引。
func (s *Server) purgeTrash(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, "项目 ID 无效")
		return
	}
	total := 0
	for {
		list, err := s.store.ListSessions(r.Context(), model.SessionFilter{
			ProjectID: projectID,
			Deleted:   true,
			Limit:     500,
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if len(list) == 0 {
			break
		}
		for _, sess := range list {
			if err := s.trash.Purge(sess); err != nil {
				writeErr(w, http.StatusInternalServerError, "清理回收站文件失败: "+err.Error())
				return
			}
			if err := s.store.HardDelete(r.Context(), sess.ID); err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			total++
		}
	}
	s.broadcast("scan")
	writeJSON(w, http.StatusOK, map[string]any{"purged": total})
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, err := s.store.GetSession(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "会话不存在")
		return
	}
	permanent := r.URL.Query().Get("permanent") == "1"

	if sess.State == "deleted" {
		if !permanent {
			writeErr(w, http.StatusConflict, "会话已经在回收站中")
			return
		}
		if err := s.trash.Purge(sess); err != nil {
			writeErr(w, http.StatusInternalServerError, "清理回收站文件失败: "+err.Error())
			return
		}
		if err := s.store.HardDelete(r.Context(), id); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "permanent": true})
		return
	}

	if opener.SessionBusy(s.opener.SessionsDir(), id) {
		writeErr(w, http.StatusConflict, opener.ErrBusy.Error())
		return
	}
	if err := s.trash.Move(sess); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.store.MarkDeleted(r.Context(), id, time.Now().Unix()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcast("scan")
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "permanent": false})
}

func (s *Server) batchDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs       []string `json:"ids"`
		Permanent bool     `json:"permanent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体不是合法 JSON")
		return
	}
	if len(body.IDs) == 0 || len(body.IDs) > 100 {
		writeErr(w, http.StatusBadRequest, "一次可选择 1–100 个会话")
		return
	}

	type failure struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	}
	failures := make([]failure, 0)
	deleted := 0
	seen := make(map[string]bool, len(body.IDs))

	for _, id := range body.IDs {
		if seen[id] {
			continue
		}
		seen[id] = true

		sess, err := s.store.GetSession(r.Context(), id)
		if err != nil {
			failures = append(failures, failure{id, "会话不存在"})
			continue
		}
		if sess.State == "deleted" {
			if !body.Permanent {
				failures = append(failures, failure{id, "会话已在回收站"})
				continue
			}
			if err := s.trash.Purge(sess); err != nil {
				failures = append(failures, failure{id, "清理文件失败"})
				continue
			}
			if err := s.store.HardDelete(r.Context(), id); err != nil {
				failures = append(failures, failure{id, "删除索引失败"})
				continue
			}
			deleted++
			continue
		}
		if body.Permanent {
			failures = append(failures, failure{id, "请先移到回收站"})
			continue
		}
		if opener.SessionBusy(s.opener.SessionsDir(), id) {
			failures = append(failures, failure{id, opener.ErrBusy.Error()})
			continue
		}
		if err := s.trash.Move(sess); err != nil {
			failures = append(failures, failure{id, err.Error()})
			continue
		}
		if err := s.store.MarkDeleted(r.Context(), id, time.Now().Unix()); err != nil {
			failures = append(failures, failure{id, err.Error()})
			continue
		}
		deleted++
	}

	s.broadcast("scan")
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted, "failed": failures})
}

func (s *Server) restoreSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, err := s.store.GetSession(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "会话不存在")
		return
	}
	if sess.State != "deleted" {
		writeErr(w, http.StatusConflict, "会话不在回收站中")
		return
	}
	if err := s.trash.Restore(sess); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.store.MarkRestored(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.broadcast("scan")
	restored, err := s.store.GetSession(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"restored": true})
		return
	}
	writeJSON(w, http.StatusOK, restored)
}

type dirItem struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (s *Server) listDirs(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("path"))
	if raw == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"path": "", "parent": "", "dirs": listRoots(),
		})
		return
	}
	if !filepath.IsAbs(raw) {
		writeErr(w, http.StatusBadRequest, "请输入绝对路径")
		return
	}
	abs := filepath.Clean(raw)
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		writeErr(w, http.StatusBadRequest, "目录不存在或不可访问")
		return
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		writeErr(w, http.StatusForbidden, "无法读取目录: "+err.Error())
		return
	}

	dirs := make([]dirItem, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirs = append(dirs, dirItem{Name: entry.Name(), Path: filepath.Join(abs, entry.Name())})
		if len(dirs) >= 500 {
			break
		}
	}
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})

	parent := filepath.Dir(abs)
	if parent == abs {
		parent = ""
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": abs, "parent": parent, "dirs": dirs})
}

func listRoots() []dirItem {
	if runtime.GOOS == "windows" {
		roots := make([]dirItem, 0, 26)
		for c := 'C'; c <= 'Z'; c++ {
			p := string(c) + ":\\"
			if info, err := os.Stat(p); err == nil && info.IsDir() {
				roots = append(roots, dirItem{Name: p, Path: p})
			}
		}
		return roots
	}
	return []dirItem{{Name: "/", Path: "/"}}
}

func (s *Server) hookRefresh(w http.ResponseWriter, r *http.Request) {
	s.hookMu.Lock()
	if time.Since(s.lastHook) < 500*time.Millisecond {
		s.hookMu.Unlock()
		writeJSON(w, http.StatusAccepted, map[string]any{"queued": true, "debounced": true})
		return
	}
	s.lastHook = time.Now()
	s.hookMu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		s.syncMu.Lock()
		res, err := s.scanner.Scan(ctx)
		s.syncMu.Unlock()
		if err != nil {
			log.Printf("hook 扫描失败: %v", err)
			return
		}
		s.SetScanState(time.Now(), res)
		s.broadcast("scan")
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": true})
}

func (s *Server) hooksConfig(w http.ResponseWriter, r *http.Request) {
	base := "http://" + r.Host
	command := "curl -s -X POST " + base + "/api/hooks/refresh >/dev/null"
	writeJSON(w, http.StatusOK, map[string]any{
		"hooks": map[string]any{
			"Stop":         []any{hookEntry(command)},
			"SessionStart": []any{hookEntry(command)},
			"SessionEnd":   []any{hookEntry(command)},
		},
	})
}

func hookEntry(command string) map[string]any {
	return map[string]any{
		"hooks": []any{map[string]any{"type": "command", "command": command}},
	}
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "当前连接不支持事件流")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan string, 4)
	s.subMu.Lock()
	s.subs[ch] = struct{}{}
	s.subMu.Unlock()
	defer func() {
		s.subMu.Lock()
		delete(s.subs, ch)
		s.subMu.Unlock()
	}()

	fmt.Fprint(w, "event: ready\ndata: ok\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev, ev)
			flusher.Flush()
		}
	}
}

func (s *Server) broadcast(ev string) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (s *Server) openSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, err := s.store.GetSession(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "会话不存在")
		return
	}
	if sess.State == "missing" {
		writeErr(w, http.StatusNotFound, "会话文件已不存在，可能已被 Claude Code 清理")
		return
	}
	if _, err := os.Stat(sess.FilePath); err != nil {
		writeErr(w, http.StatusNotFound, "会话文件已不存在")
		return
	}
	result, err := s.opener.Open(sess.Cwd, sess.ID)
	if err != nil {
		switch {
		case errors.Is(err, opener.ErrInvalidID), errors.Is(err, opener.ErrInvalidCwd):
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, opener.ErrBusy):
			writeErr(w, http.StatusConflict, err.Error())
		case errors.Is(err, opener.ErrNoClaude), errors.Is(err, opener.ErrNoTerminal):
			writeErr(w, http.StatusServiceUnavailable, err.Error())
		default:
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"opened": true, "result": result})
}

// exportSession 导出会话：json 返回原始 jsonl 内容，md 返回渲染后的 Markdown。
func (s *Server) exportSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, err := s.store.GetSession(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "会话不存在")
		return
	}
	if _, err := os.Stat(sess.FilePath); err != nil {
		writeErr(w, http.StatusNotFound, "会话文件已不存在")
		return
	}

	var data []byte
	var ext string
	switch format := r.URL.Query().Get("format"); format {
	case "json":
		data, err = os.ReadFile(sess.FilePath)
		ext = "jsonl"
		if err == nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
	case "md":
		var msgs []model.Message
		msgs, err = scanner.ReadAllMessages(sess.FilePath)
		if err == nil {
			data = []byte(sessionMarkdown(sess, msgs))
			ext = "md"
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		}
	default:
		writeErr(w, http.StatusBadRequest, "不支持的导出格式")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取会话失败: "+err.Error())
		return
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
		"filename": exportFilename(sess, ext),
	}))
	w.Write(data)
}

func exportFilename(sess model.Session, ext string) string {
	name := strings.Map(func(r rune) rune {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|', '\n', '\r', '\t':
			return '-'
		}
		return r
	}, sess.Title)
	name = strings.TrimSpace(name)
	if name == "" {
		name = "session"
	}
	if runes := []rune(name); len(runes) > 60 {
		name = string(runes[:60])
	}
	short := sess.ID
	if len(short) > 8 {
		short = short[:8]
	}
	return name + "-" + short + "." + ext
}

func sessionMarkdown(sess model.Session, msgs []model.Message) string {
	var b strings.Builder
	b.WriteString("# " + sess.Title + "\n\n")
	b.WriteString("- 会话 ID: `" + sess.ID + "`\n")
	if sess.Cwd != "" {
		b.WriteString("- 工作目录: `" + sess.Cwd + "`\n")
	}
	if sess.GitBranch != "" {
		b.WriteString("- Git 分支: `" + sess.GitBranch + "`\n")
	}
	b.WriteString("- 消息数: " + strconv.Itoa(sess.MessageCount) + "\n")
	b.WriteString("- 导出时间: " + time.Now().Format("2006-01-02 15:04:05") + "\n\n---\n\n")
	for _, m := range msgs {
		role := "用户"
		if m.Type == "assistant" {
			role = "助手"
		}
		b.WriteString("## " + role)
		if m.Timestamp != "" {
			b.WriteString(" · " + m.Timestamp)
		}
		b.WriteString("\n\n")
		if m.Text != "" {
			b.WriteString(m.Text + "\n\n")
		}
		for _, t := range m.Tools {
			b.WriteString("> 工具调用 `" + t.Name + "`\n\n```json\n" + t.Input + "\n```\n\n")
		}
	}
	return b.String()
}

func (s *Server) sync(w http.ResponseWriter, r *http.Request) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	res, err := s.scanner.Scan(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "扫描失败: "+err.Error())
		return
	}
	s.lastScan = time.Now()
	s.lastStat = res
	log.Printf("scan: %d projects, %d sessions, %d skipped, %s", res.Projects, res.Sessions, res.Skipped, res.Duration)
	s.broadcast("scan")
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) NotifyScan() {
	s.broadcast("scan")
}

func (s *Server) SetScanState(t time.Time, res scanner.Result) {
	s.lastScan = t
	s.lastStat = res
}

func pathID(r *http.Request, key string) (int64, bool) {
	return pathQueryID(r, key)
}

func pathQueryID(r *http.Request, key string) (int64, bool) {
	raw := r.PathValue(key)
	if raw == "" {
		raw = r.URL.Query().Get(key)
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host == r.Host || u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
