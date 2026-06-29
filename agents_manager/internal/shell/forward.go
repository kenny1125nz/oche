package shell

import (
	"strings"
	"time"

	"github.com/anomalyco/agents_manager/internal/agent"
	"github.com/anomalyco/agents_manager/internal/pool"
	"github.com/anomalyco/agents_manager/internal/session"
)

func buildAgentArgs(reg *agent.Registration, input string) (args []string, stdinInput string) {
	if cfg, ok := agent.ProviderFor(reg.ID); ok {
		if cfg.UseStdin {
			return nil, input
		}
		args := make([]string, len(cfg.ArgsTemplate))
		for i, t := range cfg.ArgsTemplate {
			args[i] = strings.ReplaceAll(t, "{input}", input)
		}
		return args, ""
	}
	return nil, input
}

func (r *Router) forwardToAgent(sessionID, input string) <-chan pool.ResponseEvent {
	sess, err := r.store.Get(sessionID)
	if err != nil {
		ch := make(chan pool.ResponseEvent, 5)
		emit(ch, "error", err.Error())
		ch <- pool.ResponseEvent{Type: "deliverable", Text: "done", Timestamp: time.Now()}
		close(ch)
		return ch
	}

	if sess.AgentID == "" {
		ch := make(chan pool.ResponseEvent, 5)
		emit(ch, "error", "no agent selected (use 'agents' to list, 'use <agent>' to select)")
		ch <- pool.ResponseEvent{Type: "deliverable", Text: "done", Timestamp: time.Now()}
		close(ch)
		return ch
	}

	reg, err := r.agents.Get(sess.AgentID)
	if err != nil {
		ch := make(chan pool.ResponseEvent, 5)
		emit(ch, "error", err.Error())
		ch <- pool.ResponseEvent{Type: "deliverable", Text: "done", Timestamp: time.Now()}
		close(ch)
		return ch
	}

	r.store.AddHistory(sessionID, session.Request{
		Text:        input,
		WorkspaceID: sess.WorkspaceID,
		AgentID:     sess.AgentID,
		StartedAt:   time.Now(),
	})

	workDir := sess.EffectiveDir(r.workspaces)

	agentArgs, stdinInput := buildAgentArgs(reg, input)

	cmdStr := reg.ID
	for _, a := range agentArgs {
		if strings.Contains(a, " ") {
			cmdStr += " \"" + a + "\""
		} else {
			cmdStr += " " + a
		}
	}
	if workDir != "" {
		cmdStr += "  (workdir: " + workDir + ")"
	}

	poolCh, err := r.pool.Submit(sessionID, reg.Binary, agentArgs, workDir, stdinInput)
	if err != nil {
		ech := make(chan pool.ResponseEvent, 5)
		emit(ech, "error", err.Error())
		ech <- pool.ResponseEvent{Type: "deliverable", Text: "done", Timestamp: time.Now()}
		close(ech)
		return ech
	}

	ch := make(chan pool.ResponseEvent, 256)
	emit(ch, "progress", "exec: "+cmdStr)
	go func() {
		for evt := range poolCh {
			ch <- evt
		}
		close(ch)
	}()
	return ch
}
