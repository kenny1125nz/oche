package shell

import (
	"strings"
	"sync"

	"github.com/anomalyco/agents_manager/internal/agent"
	"github.com/anomalyco/agents_manager/internal/pool"
	"github.com/anomalyco/agents_manager/internal/session"
	"github.com/anomalyco/agents_manager/internal/skill"
	"github.com/anomalyco/agents_manager/internal/workspace"
)

type CommandHandler func(sessionID string, args []string) <-chan pool.ResponseEvent

type Router struct {
	store       session.Storer
	workspaces  workspace.Registry
	agents      agent.Registry
	skills      *skill.Registry
	pool        pool.Pooler
	commands    map[string]CommandHandler
	osProcesses sync.Map
}

func NewRouter(
	store session.Storer,
	workspaces workspace.Registry,
	agents agent.Registry,
	skills *skill.Registry,
	pool pool.Pooler,
) *Router {
	r := &Router{
		store:      store,
		workspaces: workspaces,
		agents:     agents,
		skills:     skills,
		pool:       pool,
		commands:   make(map[string]CommandHandler),
	}
	r.registerDefaults()
	return r
}

func (r *Router) Register(name string, handler CommandHandler) {
	r.commands[name] = handler
}

func (r *Router) registerDefaults() {
	r.Register("help", func(sessionID string, args []string) <-chan pool.ResponseEvent {
		return wrapDone(r.cmdHelp())
	})
	r.Register("workspaces", func(sessionID string, args []string) <-chan pool.ResponseEvent {
		return wrapDone(r.cmdWorkspaces())
	})
	r.Register("ws", func(sessionID string, args []string) <-chan pool.ResponseEvent {
		return wrapDone(r.cmdWorkspaces())
	})
	r.Register("ls", func(sessionID string, args []string) <-chan pool.ResponseEvent {
		return wrapDone(r.cmdLs(sessionID, args))
	})
	r.Register("cd", func(sessionID string, args []string) <-chan pool.ResponseEvent {
		return wrapDone(r.cmdCd(sessionID, args))
	})
	r.Register("open", func(sessionID string, args []string) <-chan pool.ResponseEvent {
		return wrapDone(r.cmdOpen(sessionID, args))
	})
	r.Register("pwd", func(sessionID string, args []string) <-chan pool.ResponseEvent {
		return wrapDone(r.cmdPwd(sessionID))
	})
	r.Register("ps", func(sessionID string, args []string) <-chan pool.ResponseEvent {
		return wrapDone(r.cmdPs(sessionID))
	})
	r.Register("use", func(sessionID string, args []string) <-chan pool.ResponseEvent {
		return wrapDone(r.cmdUse(sessionID, args))
	})
	r.Register("agents", func(sessionID string, args []string) <-chan pool.ResponseEvent {
		return wrapDone(r.cmdAgents())
	})
	r.Register("skills", func(sessionID string, args []string) <-chan pool.ResponseEvent {
		return wrapDone(r.cmdSkills(sessionID))
	})
	r.Register("kill", func(sessionID string, args []string) <-chan pool.ResponseEvent {
		return wrapDone(r.cmdKill(sessionID, args))
	})
	r.Register("history", func(sessionID string, args []string) <-chan pool.ResponseEvent {
		return wrapDone(r.cmdHistory(sessionID))
	})
	r.Register("os", r.cmdOs)
	r.Register("repl", func(sessionID string, args []string) <-chan pool.ResponseEvent {
		return r.cmdRepl(sessionID)
	})
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

	if handler, ok := r.commands[cmd]; ok {
		return handler(sessionID, args)
	}
	return r.forwardToAgent(sessionID, input)
}
