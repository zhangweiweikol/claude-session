// Author: zhangweiwei
package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	aisvc "claude-session/internal/ai"
	"claude-session/internal/api"
	"claude-session/internal/creator"
	"claude-session/internal/opener"
	"claude-session/internal/scanner"
	"claude-session/internal/store"
	"claude-session/internal/trash"
	"claude-session/web"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:7788", "监听地址")
	dbPath := flag.String("db", "data/sessions.db", "SQLite 数据库路径")
	projectsDir := flag.String("projects", "", "Claude Code 会话目录，默认 ~/.claude/projects")
	flag.Parse()

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("获取用户目录失败: %v", err)
	}
	if *projectsDir == "" {
		*projectsDir = filepath.Join(home, ".claude", "projects")
	}
	sessionsDir := filepath.Join(home, ".claude", "sessions")

	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o755); err != nil {
		log.Fatalf("创建数据库目录失败: %v", err)
	}

	// GUI 子系统下没有控制台，日志同时写入 data/session-deck.log。
	logFile, err := os.OpenFile(filepath.Join(filepath.Dir(*dbPath), "session-deck.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		defer logFile.Close()
		log.SetOutput(io.MultiWriter(ignoreWriteError{os.Stderr}, logFile))
	}
	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer st.Close()

	sc := scanner.New(*projectsDir, st)
	op := opener.New(sessionsDir)
	cr := creator.New(*projectsDir, op.ClaudePath())
	tr := trash.New(filepath.Join(home, ".claude-session", "trash"))

	// AI 摘要通过本机 Claude Code 生成，临时目录固定，避免污染项目扫描。
	aiWorkDir := filepath.Join(filepath.Dir(*dbPath), ".ai-tmp")
	gen := aisvc.NewCLIGenerator(op.ClaudePath(), "", aiWorkDir)
	var server *api.Server
	aiCfg := aisvc.LoadSettings(context.Background(), st)
	gen.SetModel(aiCfg.Model)
	aiService := aisvc.New(st, gen, func(string) {
		if server != nil {
			server.NotifyAI()
		}
	})
	server = api.New(st, sc, op, cr, tr, aiService, web.FS)
	aiService.Start()

	if !gen.Available() {
		log.Printf("未找到 claude 可执行文件，AI 功能不可用")
	}

	// 首次启用全文索引时，强制重新解析已有会话。
	if n, err := st.CountFTS(context.Background()); err == nil && n == 0 {
		if total, _ := st.CountSessions(context.Background()); total > 0 {
			if err := st.ResetFileState(context.Background()); err != nil {
				log.Printf("重置会话解析状态失败: %v", err)
			}
		}
	}

	start := time.Now()
	res, err := sc.Scan(context.Background())
	if err != nil {
		log.Printf("初始扫描失败: %v", err)
	} else {
		server.SetScanState(start, res)
		log.Printf("初始扫描完成: %d 个项目, %d 个会话, %d 个跳过, 耗时 %s",
			res.Projects, res.Sessions, res.Skipped, res.Duration.Round(time.Millisecond))
	}

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			res, err := sc.Scan(ctx)
			cancel()
			if err != nil {
				log.Printf("定时扫描失败: %v", err)
				continue
			}
			server.SetScanState(time.Now(), res)
			server.NotifyScan()
		}
	}()

	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("会话台已启动: http://%s", *addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("服务启动失败: %v", err)
	}
}

type ignoreWriteError struct{ w io.Writer }

func (w ignoreWriteError) Write(p []byte) (int, error) {
	_, _ = w.w.Write(p)
	return len(p), nil
}
