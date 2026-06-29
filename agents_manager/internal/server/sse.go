package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/anomalyco/agents_manager/internal/pool"
	"github.com/anomalyco/agents_manager/internal/session"
)

type sseHandler struct {
	store *session.Store
	pool  *pool.Pool
}

func (h *sseHandler) dashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	send := func(data any) {
		raw, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", raw)
		flusher.Flush()
	}

	// Initial state
	h.sendPoolState(send)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			h.sendPoolState(send)
		}
	}
}

func (h *sseHandler) sendPoolState(send func(any)) {
	sessions := h.store.List()
	sessionData := make([]map[string]any, 0, len(sessions))
	for _, s := range sessions {
		active := h.pool.HasActive(s.ID)
		sessionData = append(sessionData, map[string]any{
			"id":            s.ID,
			"workspace_id":  s.WorkspaceID,
			"sub_workspace": s.SubWorkspace,
			"active_file":   s.ActiveFile,
			"agent_id":      s.AgentID,
			"state":         s.State,
			"active":        active,
		})
	}

	send(map[string]any{
		"type":     "dashboard",
		"sessions": sessionData,
		"pool": map[string]any{
			"active": h.pool.ActiveCount(),
			"queued": h.pool.QueuedCount(),
		},
		"timestamp": time.Now(),
	})
}

func sseSetupHandler(w http.ResponseWriter, r *http.Request) {
	data, _ := json.Marshal(pool.ResponseEvent{
		Type:      "setup_required",
		Text:      "password not set",
		Timestamp: time.Now(),
	})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	fmt.Fprintf(w, "data: %s\n\n", data)
}
