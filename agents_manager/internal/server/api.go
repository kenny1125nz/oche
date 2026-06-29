package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/anomalyco/agents_manager/internal/agent"
	"github.com/anomalyco/agents_manager/internal/pool"
	"github.com/anomalyco/agents_manager/internal/session"
	"github.com/anomalyco/agents_manager/internal/workspace"
)

type apiHandler struct {
	sessions   *session.Store
	workspaces *workspace.Registry
	agents     *agent.Registry
	pool       *pool.Pool
}

func (h *apiHandler) sessionsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	list := h.sessions.List()
	data := make([]map[string]any, 0, len(list))
	for _, s := range list {
		data = append(data, map[string]any{
			"id":            s.ID,
			"transport":     s.Transport,
			"workspace_id":  s.WorkspaceID,
			"sub_workspace": s.SubWorkspace,
			"active_file":   s.ActiveFile,
			"agent_id":      s.AgentID,
			"state":         s.State,
		})
	}
	json.NewEncoder(w).Encode(data)
}

func (h *apiHandler) workspacesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	list := h.workspaces.List()
	data := make([]map[string]any, 0, len(list))
	for _, ws := range list {
		data = append(data, map[string]any{
			"id":         ws.ID,
			"target_dir": ws.TargetDir,
		})
	}
	json.NewEncoder(w).Encode(data)
}

func (h *apiHandler) workspaceTreeHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/workspace/")
	id = strings.TrimSuffix(id, "/tree")

	ws, err := h.workspaces.Get(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	depth := 2
	if d := r.URL.Query().Get("depth"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v > 0 {
			depth = v
		}
	}

	tree := ws.Tree(depth)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":         ws.ID,
		"target_dir": ws.TargetDir,
		"tree":       tree,
	})
}

func (h *apiHandler) sessionActiveFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/session/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[1] != "active-file" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	sessionID := parts[0]

	sess, err := h.sessions.Get(sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var req struct {
		File string `json:"file"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	sess.ActiveFile = req.File
	if err := h.sessions.Update(sess); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (h *apiHandler) agentsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	list := h.agents.List()
	data := make([]map[string]any, 0, len(list))
	for _, reg := range list {
		data = append(data, map[string]any{
			"id":     reg.ID,
			"binary": reg.Binary,
			"system": reg.System,
		})
	}
	json.NewEncoder(w).Encode(data)
}

func (h *apiHandler) poolHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"active": h.pool.ActiveCount(),
		"queued": h.pool.QueuedCount(),
	})
}

func parseAgentPID(path string) (int, error) {
	trimmed := strings.TrimPrefix(path, "/api/agent/")
	parts := strings.SplitN(trimmed, "/", 2)
	return strconv.Atoi(parts[0])
}

func (h *apiHandler) activeAgentsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.pool.ActiveList())
}

func (h *apiHandler) agentInputHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pid, err := parseAgentPID(r.URL.Path)
	if err != nil {
		log.Printf("[agent-input] invalid pid: %v", err)
		http.Error(w, "invalid pid", http.StatusBadRequest)
		return
	}

	var req struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[agent-input] invalid body: %v", err)
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	log.Printf("[agent-input] pid=%d input=%q", pid, truncate(req.Input, 80))
	if err := h.pool.SendInput(pid, req.Input); err != nil {
		log.Printf("[agent-input] send failed pid=%d: %v", pid, err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	log.Printf("[agent-input] sent to pid=%d", pid)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "sent"})
}

func (h *apiHandler) agentStreamHandler(w http.ResponseWriter, r *http.Request) {
	pid, err := parseAgentPID(r.URL.Path)
	if err != nil {
		log.Printf("[agent-stream] invalid pid: %v", err)
		http.Error(w, "invalid pid", http.StatusBadRequest)
		return
	}

	ag := h.pool.GetAgent(pid)
	if ag == nil {
		log.Printf("[agent-stream] agent %d not found", pid)
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	log.Printf("[agent-stream] agent %d found, mode=%s", pid, ag.Mode)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ch, err := h.pool.SubscribeAgent(pid)
	if err != nil {
		log.Printf("[agent-stream] subscribe failed for pid %d: %v", pid, err)
		return
	}
	log.Printf("[agent-stream] subscribed for pid %d", pid)
	defer h.pool.UnsubscribeAgent(pid, ch)

	drainChan := func() {
		for {
			select {
			case line := <-ch:
				log.Printf("[agent-stream] drain received %q for pid %d", truncate(line, 80), pid)
				if line == "__agent_exit__" {
					fmt.Fprintf(w, "data: %s\n\n", jsonString(map[string]string{"type": "exit", "text": "done"}))
					flusher.Flush()
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", jsonString(map[string]string{"type": "stdout", "text": line}))
				flusher.Flush()
			default:
				return
			}
		}
	}

	drainChan()

	stdoutLines := ag.StdoutLines()
	log.Printf("[agent-stream] replaying %d stdout lines for pid %d", len(stdoutLines), pid)
	for _, line := range stdoutLines {
		fmt.Fprintf(w, "data: %s\n\n", jsonString(map[string]string{"type": "stdout", "text": line}))
	}

	stderrLines := ag.StderrLines()
	log.Printf("[agent-stream] replaying %d stderr lines for pid %d", len(stderrLines), pid)
	for _, line := range stderrLines {
		fmt.Fprintf(w, "data: %s\n\n", jsonString(map[string]string{"type": "stderr", "text": line}))
	}
	flusher.Flush()
	log.Printf("[agent-stream] history done for pid %d, entering live loop", pid)

	for {
		select {
		case line, ok := <-ch:
			if !ok {
				log.Printf("[agent-stream] channel closed for pid %d", pid)
				return
			}
			log.Printf("[agent-stream] live received %q for pid %d", truncate(line, 80), pid)
			if line == "__agent_exit__" {
				log.Printf("[agent-stream] exit for pid %d", pid)
				fmt.Fprintf(w, "data: %s\n\n", jsonString(map[string]string{"type": "exit", "text": "done"}))
				flusher.Flush()
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", jsonString(map[string]string{"type": "stdout", "text": line}))
			flusher.Flush()
		case <-r.Context().Done():
			log.Printf("[agent-stream] client disconnected for pid %d", pid)
			return
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func jsonString(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func (h *apiHandler) processStdinHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pid, err := parseProcessPID(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid pid", http.StatusBadRequest)
		return
	}

	var req struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if err := h.pool.SendInput(pid, req.Input); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

func parseProcessPID(path string) (int, error) {
	trimmed := strings.TrimPrefix(path, "/api/process/")
	parts := strings.SplitN(trimmed, "/", 2)
	return strconv.Atoi(parts[0])
}

func (h *apiHandler) agentOutputHandler(w http.ResponseWriter, r *http.Request) {
	pid, err := parseAgentPID(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid pid", http.StatusBadRequest)
		return
	}

	ag := h.pool.GetAgent(pid)
	if ag == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"pid":    ag.PID,
		"state":  ag.State,
		"stdout": strings.Join(ag.StdoutLines(), "\n"),
		"stderr": strings.Join(ag.StderrLines(), "\n"),
		"exit":   ag.ExitCode,
	})
}
