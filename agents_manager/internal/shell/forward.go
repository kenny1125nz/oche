package shell

import (
	"strings"
	"time"

	"github.com/anomalyco/agents_manager/internal/agent"
	"github.com/anomalyco/agents_manager/internal/pool"
	"github.com/anomalyco/agents_manager/internal/session"
)

func buildAgentArgs(reg *agent.Registration, input string, providers agent.Registry) (args []string, stdinInput string) {
	if cfg, ok := providers.ProviderFor(reg.ID); ok {
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

func newErrorResponse(msg string) <-chan pool.ResponseEvent {
	ch := make(chan pool.ResponseEvent, 5)
	emit(ch, "error", msg)
	ch <- pool.ResponseEvent{Type: "deliverable", Text: "done", Timestamp: time.Now()}
	close(ch)
	return ch
}

func (r *Router) forwardToAgent(sessionID, input string) <-chan pool.ResponseEvent {
	sess, err := r.store.Get(sessionID)
	if err != nil {
		return newErrorResponse(err.Error())
	}

	if sess.AgentID == "" {
		return newErrorResponse("no agent selected (use 'agents' to list, 'use <agent>' to select)")
	}

	reg, err := r.agents.Get(sess.AgentID)
	if err != nil {
		return newErrorResponse(err.Error())
	}

	r.store.AddHistory(sessionID, session.Request{
		Text:        input,
		WorkspaceID: sess.WorkspaceID,
		AgentID:     sess.AgentID,
		StartedAt:   time.Now(),
	})

	workDir := sess.EffectiveDir(r.workspaces)

	agentArgs, stdinInput := buildAgentArgs(reg, input, r.agents)

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
		return newErrorResponse(err.Error())
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
