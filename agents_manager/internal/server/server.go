package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/anomalyco/agents_manager/internal/agent"
	"github.com/anomalyco/agents_manager/internal/auth"
	"github.com/anomalyco/agents_manager/internal/pool"
	"github.com/anomalyco/agents_manager/internal/session"
	"github.com/anomalyco/agents_manager/internal/shell"
	"github.com/anomalyco/agents_manager/internal/skill"
	"github.com/anomalyco/agents_manager/internal/workspace"
)

type Config struct {
	Port        int
	Bind        string
	StaticDir   string
	AuthManager *auth.Manager
	Sessions    *session.Store
	Workspaces  *workspace.Registry
	Agents      *agent.Registry
	Skills      *skill.Registry
	Pool        *pool.Pool
	Router      *shell.Router
}

func Start(cfg Config) error {
	mux := http.NewServeMux()

	// Auth endpoints
	authMgr := cfg.AuthManager
	mux.HandleFunc("/auth/status", authMgr.StatusHandler)
	mux.HandleFunc("/auth/login", authMgr.Login)
	mux.HandleFunc("/auth/refresh", authMgr.Refresh)
	mux.HandleFunc("/auth/setup", authMgr.SetupHandler)

	// API endpoints
	api := &apiHandler{
		sessions:   cfg.Sessions,
		workspaces: cfg.Workspaces,
		agents:     cfg.Agents,
		pool:       cfg.Pool,
	}
	mux.HandleFunc("/api/sessions", api.sessionsHandler)
	mux.HandleFunc("/api/workspaces", api.workspacesHandler)
	mux.HandleFunc("/api/workspace/", api.workspaceTreeHandler)
	mux.HandleFunc("/api/session/", api.sessionActiveFileHandler)
	mux.HandleFunc("/api/agents", api.agentsHandler)
	mux.HandleFunc("/api/pool", api.poolHandler)
	mux.HandleFunc("/api/agents/active", api.activeAgentsHandler)

	// Stdin injection via REST: POST /api/process/<pid>/stdin
	mux.HandleFunc("/api/process/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/stdin") {
			api.processStdinHandler(w, r)
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	// Agent interaction: /api/agent/<pid>/input, /api/agent/<pid>/stream, /api/agent/<pid>/output
	mux.HandleFunc("/api/agent/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/input") {
			api.agentInputHandler(w, r)
		} else if strings.HasSuffix(path, "/stream") {
			api.agentStreamHandler(w, r)
		} else if strings.HasSuffix(path, "/output") {
			api.agentOutputHandler(w, r)
		} else {
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	// WebSocket shell
	ws := &wsHandler{router: cfg.Router, store: cfg.Sessions, pool: cfg.Pool}
	mux.HandleFunc("/shell", ws.serve)

	// WebSocket per-agent stream
	wa := &wsAgentHandler{pool: cfg.Pool}
	mux.HandleFunc("/ws/agent/", wa.serve)

	// SSE dashboard
	sse := &sseHandler{store: cfg.Sessions, pool: cfg.Pool}
	mux.HandleFunc("/sse/dashboard", sse.dashboard)

	// Static files (SPA)
	staticDir := cfg.StaticDir
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		os.MkdirAll(staticDir, 0755)
	}
	fileServer := http.FileServer(http.Dir(staticDir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	var handler http.Handler = mux
	handler = authMiddleware(authMgr, handler)
	handler = setupCheckMiddleware(authMgr, handler)

	addr := fmt.Sprintf("%s:%d", cfg.Bind, cfg.Port)
	log.Printf("HTTP server listening on %s", addr)
	return http.ListenAndServe(addr, handler)
}
