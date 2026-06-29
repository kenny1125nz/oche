package shell

import (
	"time"

	"github.com/anomalyco/agents_manager/internal/pool"
)

// emit sends a display event. Each call represents one line of output, so
// a trailing newline is added automatically. This keeps line breaks
// consistent across all built-in commands without per-caller \n, and
// avoids special-casing on the frontend. Binary PTY output (cmdOs /
// pool.monitor) sends events directly and is unaffected.
func emit(ch chan pool.ResponseEvent, typ, text string) {
	ch <- pool.ResponseEvent{
		Type:      typ,
		Text:      text + "\n",
		Timestamp: time.Now(),
	}
}

func emitSession(ch chan pool.ResponseEvent, typ, text, workspaceID, agentID string) {
	ch <- pool.ResponseEvent{
		Type:        typ,
		Text:        text + "\n",
		WorkspaceID: workspaceID,
		AgentID:     agentID,
		Timestamp:   time.Now(),
	}
}

func wrapDone(in <-chan pool.ResponseEvent) <-chan pool.ResponseEvent {
	out := make(chan pool.ResponseEvent, 256)
	go func() {
		for evt := range in {
			out <- evt
		}
		out <- pool.ResponseEvent{Type: "deliverable", Text: "done", Timestamp: time.Now()}
		close(out)
	}()
	return out
}
