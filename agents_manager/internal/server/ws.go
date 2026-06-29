package server

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/anomalyco/agents_manager/internal/pool"
	"github.com/anomalyco/agents_manager/internal/session"
	"github.com/anomalyco/agents_manager/internal/shell"

	gorilla "github.com/gorilla/websocket"
)

var upgrader = gorilla.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type wsHandler struct {
	router  *shell.Router
	store   *session.Store
	pool    *pool.Pool
}

func (h *wsHandler) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}
	defer conn.Close()

	sess := &session.Session{
		ID:        "ws-" + time.Now().Format("150405.000"),
		Transport: "ws",
		State:     session.StateIdle,
	}
	h.store.Create(sess)
	defer h.store.Delete(sess.ID)

	conn.WriteJSON(pool.ResponseEvent{
		SessionID: sess.ID,
		Type:      "ack",
		Text:      "connected. session: " + sess.ID,
		Timestamp: time.Now(),
	})

	var runMu sync.Mutex
	running := false

	msgs := make(chan string, 64)
	done := make(chan struct{})

	// Writer goroutine: processes commands and streams events
	go func() {
		defer close(done)
		for msg := range msgs {
			runMu.Lock()
			running = true
			runMu.Unlock()

			ch := h.router.Execute(sess.ID, msg)
			for evt := range ch {
				conn.WriteJSON(evt)
			}

			runMu.Lock()
			running = false
			runMu.Unlock()
		}
	}()

	// Reader goroutine: reads WS messages, routes input or dispatches commands
	go func() {
		defer close(msgs)
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
				conn.WriteJSON(pool.ResponseEvent{
					Type: "error",
					Text: "invalid request format",
				})
				continue
			}

			if req.Resize {
				// Store the terminal size on the session so cmdOs can read
				// it when starting an OS process with pty.StartWithSize.
				if s, err := h.store.Get(sess.ID); err == nil {
					s.TermCols = req.Cols
					s.TermRows = req.Rows
					h.store.Update(s)
				}
				if shell.HasOSProcess(sess.ID) {
					shell.ResizeOSProcess(sess.ID, req.Rows, req.Cols)
				} else if ag := h.pool.GetAgentBySession(sess.ID); ag != nil {
					ag.Resize(req.Rows, req.Cols)
				}
				continue
			}

			runMu.Lock()
			isRunning := running
			runMu.Unlock()

			if isRunning {
				// Route input to whichever type of process is active.
				// "os" commands use direct PTY writes (no newline added).
				// Pool agents (repl, forward) use WriteStdin which does
				// append \n — only call it when no os process is running.
				if shell.HasOSProcess(sess.ID) {
					shell.WriteOSInput(sess.ID, req.Input)
				} else if ag := h.pool.GetAgentBySession(sess.ID); ag != nil {
					ag.WriteStdin(req.Input)
				}
			} else {
				msgs <- req.Input
			}
		}
	}()

	<-done
}
