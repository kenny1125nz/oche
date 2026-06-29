package shell

import (
	"strings"

	"github.com/anomalyco/agents_manager/internal/agent"
	"github.com/anomalyco/agents_manager/internal/pool"
	"github.com/anomalyco/agents_manager/internal/session"
	"github.com/anomalyco/agents_manager/internal/skill"
	"github.com/anomalyco/agents_manager/internal/workspace"
)

type Router struct {
	store      *session.Store
	workspaces *workspace.Registry
	agents     *agent.Registry
	skills     *skill.Registry
	pool       *pool.Pool
}

func NewRouter(
	store *session.Store,
	workspaces *workspace.Registry,
	agents *agent.Registry,
	skills *skill.Registry,
	pool *pool.Pool,
) *Router {
	return &Router{
		store:      store,
		workspaces: workspaces,
		agents:     agents,
		skills:     skills,
		pool:       pool,
	}
}

func (r *Router) Execute(sessionID, input string) <-chan pool.ResponseEvent {
	input = strings.TrimSpace(input)
	if input == "" {
		ch := make(chan pool.ResponseEvent)
		close(ch)
		return ch
	}

	parts := strings.Fields(input)
	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "help":
		return wrapDone(r.cmdHelp())
	case "workspaces", "ws":
		return wrapDone(r.cmdWorkspaces())
	case "ls":
		return wrapDone(r.cmdLs(sessionID, args))
	case "cd":
		return wrapDone(r.cmdCd(sessionID, args))
	case "open":
		return wrapDone(r.cmdOpen(sessionID, args))
	case "pwd":
		return wrapDone(r.cmdPwd(sessionID))
	case "ps":
		return wrapDone(r.cmdPs(sessionID))
	case "use":
		return wrapDone(r.cmdUse(sessionID, args))
	case "agents":
		return wrapDone(r.cmdAgents())
	case "skills":
		return wrapDone(r.cmdSkills(sessionID))
	case "kill":
		return wrapDone(r.cmdKill(sessionID, args))
	case "history":
		return wrapDone(r.cmdHistory(sessionID))
	case "os":
		return r.cmdOs(sessionID, args)
	case "repl":
		return r.cmdRepl(sessionID)
	default:
		return r.forwardToAgent(sessionID, input)
	}
}
