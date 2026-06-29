package shell

import (
	"fmt"

	"github.com/anomalyco/agents_manager/internal/pool"
)

func (r *Router) cmdHelp() <-chan pool.ResponseEvent {
	ch := make(chan pool.ResponseEvent, 20)
	emit(ch, "deliverable", "Agents Manager built-in commands:")
	emit(ch, "progress", "  workspaces (ws)  List available workspaces")
	emit(ch, "progress", "  open <workspace> Select a workspace")
	emit(ch, "progress", "  cd <dir>         Navigate workspace sub-directories")
	emit(ch, "progress", "  pwd              Show current workspace path")
	emit(ch, "progress", "  ls [<dir>]       List directory contents")
	emit(ch, "progress", "  ps               Show active sessions/agents")
	emit(ch, "progress", "  use <agent>      Switch AI agent provider")
	emit(ch, "progress", "  agents           List available agents")
	emit(ch, "progress", "  skills           List available skills")
	emit(ch, "progress", "  os <command>     Run host OS command")
	emit(ch, "progress", "  repl             Start an interactive REPL session with the current agent")
	emit(ch, "progress", "  kill <pid>       Kill a running agent by PID or session ID")
	emit(ch, "progress", "  history          Show recent requests")
	emit(ch, "progress", "  help             Show this help")
	emit(ch, "progress", "  <anything else>  Forwards to current AI agent")
	close(ch)
	return ch
}

func (r *Router) cmdWorkspaces() <-chan pool.ResponseEvent {
	ch := make(chan pool.ResponseEvent, 50)

	if r.workspaces.Len() == 0 {
		emit(ch, "deliverable", "No workspaces configured")
		close(ch)
		return ch
	}

	emit(ch, "deliverable", "Workspaces:")
	for _, ws := range r.workspaces.List() {
		emit(ch, "progress", fmt.Sprintf("  %s  (%s)", ws.ID, ws.TargetDir))
	}

	close(ch)
	return ch
}

func (r *Router) cmdAgents() <-chan pool.ResponseEvent {
	ch := make(chan pool.ResponseEvent, 30)

	if r.agents.Len() == 0 {
		emit(ch, "deliverable", "No agents discovered. Add binaries to ~/.agents_manager/agents/")
		close(ch)
		return ch
	}

	emit(ch, "deliverable", "Available agents:")
	for _, reg := range r.agents.List() {
		suffix := ""
		if reg.System {
			suffix = " [system]"
		}
		emit(ch, "progress", fmt.Sprintf("  %s  (%s)%s", reg.ID, reg.Binary, suffix))
	}

	close(ch)
	return ch
}

func (r *Router) cmdSkills(sessionID string) <-chan pool.ResponseEvent {
	ch := make(chan pool.ResponseEvent, 50)

	sess, err := r.store.Get(sessionID)
	if err != nil {
		emit(ch, "error", err.Error())
		close(ch)
		return ch
	}

	var wsPath, agentPath string

	if sess.AgentID != "" {
		if reg, err := r.agents.Get(sess.AgentID); err == nil {
			agentPath = reg.Path
		}
	}
	wsPath = sess.EffectiveDir(r.workspaces)

	all := r.skills.GetAll(wsPath, agentPath)

	if len(all) == 0 {
		emit(ch, "deliverable", "No skills discovered")
		close(ch)
		return ch
	}

	emit(ch, "deliverable", "Available skills (agent > workspace > global):")
	for _, sk := range all {
		desc := sk.Description
		if desc == "" {
			desc = sk.Binary
		}
		emit(ch, "progress", fmt.Sprintf("  %-20s %s", sk.Name, desc))
	}

	close(ch)
	return ch
}

func (r *Router) cmdHistory(sessionID string) <-chan pool.ResponseEvent {
	ch := make(chan pool.ResponseEvent, 30)

	sess, err := r.store.Get(sessionID)
	if err != nil {
		emit(ch, "error", err.Error())
		close(ch)
		return ch
	}

	if len(sess.History) == 0 {
		emit(ch, "deliverable", "No history")
		close(ch)
		return ch
	}

	emit(ch, "deliverable", "Recent requests:")
	for i, req := range sess.History {
		prefix := fmt.Sprintf("%2d", i+1)
		if i >= 10 && i >= len(sess.History)-10 {
		} else if i < len(sess.History)-10 {
			continue
		}
		emit(ch, "progress", fmt.Sprintf("  %s  %s", prefix, req.Text))
	}

	close(ch)
	return ch
}
