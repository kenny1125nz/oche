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
	pool pool.Pooler
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
				ag.WriteStdinRaw([]byte(req.Input))
			}
		}
	}()

	streamAgentOutput(r.Context(), ch, ag, streamCallbacks{
		writeLine: func(line string) {
			writeJSON(map[string]string{"text": line})
		},
		writeExit: func() {
			writeJSON(map[string]any{"exit": true})
		},
	})
}
