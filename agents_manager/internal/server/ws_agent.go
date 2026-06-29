package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/anomalyco/agents_manager/internal/pool"

	gorilla "github.com/gorilla/websocket"
)

type wsAgentHandler struct {
	pool *pool.Pool
}

func (h *wsAgentHandler) serve(w http.ResponseWriter, r *http.Request) {
	pidStr := strings.TrimPrefix(r.URL.Path, "/ws/agent/")
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		http.Error(w, "invalid pid", http.StatusBadRequest)
		return
	}

	ag := h.pool.GetAgent(pid)
	if ag == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ch, err := h.pool.SubscribeAgent(pid)
	if err != nil {
		return
	}
	defer h.pool.UnsubscribeAgent(pid, ch)

	writeJSON := func(v any) {
		b, err := json.Marshal(v)
		if err != nil {
			return
		}
		conn.WriteMessage(gorilla.TextMessage, b)
	}

	drainChan := func() {
		for {
			select {
			case line := <-ch:
				if line == "__agent_exit__" {
					writeJSON(map[string]any{"exit": true})
					return
				}
				if line == "" {
					continue
				}
				writeJSON(map[string]string{"text": line})
			default:
				return
			}
		}
	}

	drainChan()

	// Reader goroutine: forward browser xterm.js responses (e.g. cursor
	// position reports, DECRPM answers) back to the agent's PTY stdin so
	// terminal capability negotiation works bidirectionally.
	// User keystrokes are NOT captured here — they go through the REST
	// endpoint /api/agent/<pid>/input for low-latency typing.
	go func() {
		defer conn.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req struct {
				Input  string `json:"input"`
				Resize bool   `json:"resize"`
				Cols   uint16 `json:"cols"`
				Rows   uint16 `json:"rows"`
			}
			if err := json.Unmarshal(msg, &req); err != nil {
				continue
			}
			if req.Resize {
				h.pool.ResizePTY(pid, req.Rows, req.Cols)
				continue
			}
			if req.Input != "" {
				// Raw write — no newline appended (terminal responses are
				// escape sequences that must not be altered).
				ag.WriteStdinRaw([]byte(req.Input))
			}
		}
	}()

	// Replay history
	stdout := ag.StdoutLines()
	stderr := ag.StderrLines()
	for _, line := range stdout {
		if line != "" {
			writeJSON(map[string]string{"text": line})
		}
	}
	for _, line := range stderr {
		if line != "" {
			writeJSON(map[string]string{"text": line})
		}
	}

	// Live loop
	for {
		line, ok := <-ch
		if !ok {
			return
		}
		if line == "__agent_exit__" {
			writeJSON(map[string]any{"exit": true})
			return
		}
		if line != "" {
			writeJSON(map[string]string{"text": line})
		}
	}
}
