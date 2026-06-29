package shell

import (
	"fmt"
	"strconv"
	"time"

	"github.com/anomalyco/agents_manager/internal/pool"
)

func (r *Router) cmdPs(sessionID string) <-chan pool.ResponseEvent {
	ch := make(chan pool.ResponseEvent, 20)

	emit(ch, "deliverable", "Active agents:")

	for _, a := range r.pool.ActiveList() {
		emit(ch, "progress", fmt.Sprintf("  pid=%-6d session=%-12s binary=%s", a.PID, a.SessionID, a.Binary))
	}

	emit(ch, "progress", fmt.Sprintf("pool: %d active, %d queued",
		r.pool.ActiveCount(), r.pool.QueuedCount()))

	close(ch)
	return ch
}

func (r *Router) cmdUse(sessionID string, args []string) <-chan pool.ResponseEvent {
	ch := make(chan pool.ResponseEvent, 10)

	if len(args) == 0 {
		emit(ch, "error", "usage: use <agent>")
		close(ch)
		return ch
	}

	target := args[0]
	reg, err := r.agents.Get(target)
	if err != nil {
		emit(ch, "error", err.Error())
		close(ch)
		return ch
	}

	sess, err := r.store.Get(sessionID)
	if err != nil {
		emit(ch, "error", err.Error())
		close(ch)
		return ch
	}

	sess.AgentID = reg.ID
	if err := r.store.Update(sess); err != nil {
		emit(ch, "error", err.Error())
		close(ch)
		return ch
	}

	emitSession(ch, "deliverable", fmt.Sprintf("switched to agent %s (%s)", reg.ID, reg.Binary), "", reg.ID)
	close(ch)
	return ch
}

func (r *Router) cmdKill(sessionID string, args []string) <-chan pool.ResponseEvent {
	ch := make(chan pool.ResponseEvent, 10)

	if len(args) == 0 {
		emit(ch, "error", "usage: kill <pid-or-session-id>")
		close(ch)
		return ch
	}

	target := args[0]

	if pid, err := strconv.Atoi(target); err == nil {
		if err := r.pool.KillByPID(pid); err == nil {
			emit(ch, "deliverable", fmt.Sprintf("killed agent pid %d", pid))
			close(ch)
			return ch
		}
	}

	if err := r.pool.Kill(target); err != nil {
		emit(ch, "error", err.Error())
	} else {
		emit(ch, "deliverable", fmt.Sprintf("killed agent for session %s", target))
	}

	close(ch)
	return ch
}

func (r *Router) cmdRepl(sessionID string) <-chan pool.ResponseEvent {
	ch := make(chan pool.ResponseEvent, 256)

	sess, err := r.store.Get(sessionID)
	if err != nil {
		emit(ch, "error", err.Error())
		ch <- pool.ResponseEvent{Type: "deliverable", Text: "done", Timestamp: time.Now()}
		close(ch)
		return ch
	}

	if sess.AgentID == "" {
		emit(ch, "error", "no agent selected (use 'use <agent>' first)")
		ch <- pool.ResponseEvent{Type: "deliverable", Text: "done", Timestamp: time.Now()}
		close(ch)
		return ch
	}

	if sess.WorkspaceID == "" {
		emit(ch, "error", "no workspace selected (use 'open <workspace>' first)")
		ch <- pool.ResponseEvent{Type: "deliverable", Text: "done", Timestamp: time.Now()}
		close(ch)
		return ch
	}

	reg, err := r.agents.Get(sess.AgentID)
	if err != nil {
		emit(ch, "error", err.Error())
		ch <- pool.ResponseEvent{Type: "deliverable", Text: "done", Timestamp: time.Now()}
		close(ch)
		return ch
	}

	workDir := sess.EffectiveDir(r.workspaces)
	poolID := "repl-" + sessionID

	emit(ch, "progress", fmt.Sprintf("starting REPL for %s in %s", reg.ID, workDir))

	poolCh, err := r.pool.SubmitWithMode(poolID, reg.Binary, nil, workDir, pool.PTYMode, "")
	if err != nil {
		emit(ch, "error", err.Error())
		ch <- pool.ResponseEvent{Type: "deliverable", Text: "done", Timestamp: time.Now()}
		close(ch)
		return ch
	}

	if evt, ok := <-poolCh; ok {
		ch <- evt
		go func() {
			for range poolCh {
			}
		}()
	} else {
		emit(ch, "error", "failed to start REPL process")
		ch <- pool.ResponseEvent{Type: "deliverable", Text: "done", Timestamp: time.Now()}
	}

	close(ch)
	return ch
}
