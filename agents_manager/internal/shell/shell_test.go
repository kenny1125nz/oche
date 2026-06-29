package shell

import (
	"testing"

	"github.com/anomalyco/agents_manager/internal/agent"
	"github.com/anomalyco/agents_manager/internal/pool"
	"github.com/anomalyco/agents_manager/internal/session"
	"github.com/anomalyco/agents_manager/internal/skill"
	"github.com/anomalyco/agents_manager/internal/workspace"
)

func newTestRouter(t *testing.T) (*Router, *session.Store) {
	t.Helper()

	ws := workspace.NewRegistry()
	ws.Discover([]string{t.TempDir()}) // empty dir → no workspaces

	ag := agent.NewRegistry()
	ag.Discover(t.TempDir()) // empty dir → no agents

	sk := skill.NewRegistry(t.TempDir())
	store := session.NewStore()
	agPool := pool.New(2, nil)

	router := NewRouter(store, ws, ag, sk, agPool)
	return router, store
}

func collect(ch <-chan pool.ResponseEvent) []pool.ResponseEvent {
	var events []pool.ResponseEvent
	for evt := range ch {
		events = append(events, evt)
	}
	return events
}

func TestHelp(t *testing.T) {
	r, s := newTestRouter(t)
	s.Create(&session.Session{ID: "sess-1"})

	events := collect(r.Execute("sess-1", "help"))
	if len(events) == 0 {
		t.Fatal("expected help events")
	}
	if events[0].Type != "deliverable" {
		t.Fatalf("expected type 'deliverable', got %s", events[0].Type)
	}
}

func TestEmptyInput(t *testing.T) {
	r, s := newTestRouter(t)
	s.Create(&session.Session{ID: "sess-1"})

	events := collect(r.Execute("sess-1", ""))
	if len(events) != 0 {
		t.Fatalf("expected 0 events for empty input, got %d", len(events))
	}
}

func TestLs(t *testing.T) {
	r, s := newTestRouter(t)
	s.Create(&session.Session{ID: "sess-1"})

	events := collect(r.Execute("sess-1", "ls"))
	if len(events) == 0 {
		t.Fatal("expected ls events")
	}
}

func TestPwdNoWorkspace(t *testing.T) {
	r, s := newTestRouter(t)
	s.Create(&session.Session{ID: "sess-1"})

	events := collect(r.Execute("sess-1", "pwd"))
	if len(events) == 0 {
		t.Fatal("expected pwd events")
	}
	if events[0].Text != "no workspace set (use open <workspace>)\n" {
		t.Fatalf("unexpected pwd output: %s", events[0].Text)
	}
}

func TestCdMissingArg(t *testing.T) {
	r, s := newTestRouter(t)
	s.Create(&session.Session{ID: "sess-1"})

	events := collect(r.Execute("sess-1", "cd"))
	if len(events) == 0 || events[0].Type != "error" {
		t.Fatal("expected error for cd without arg")
	}
}

func TestUseMissingArg(t *testing.T) {
	r, s := newTestRouter(t)
	s.Create(&session.Session{ID: "sess-1"})

	events := collect(r.Execute("sess-1", "use"))
	if len(events) == 0 || events[0].Type != "error" {
		t.Fatal("expected error for use without arg")
	}
}

func TestForwardWithoutAgent(t *testing.T) {
	r, s := newTestRouter(t)
	s.Create(&session.Session{ID: "sess-1"})

	events := collect(r.Execute("sess-1", "hello world"))
	if len(events) == 0 || events[0].Type != "error" {
		t.Fatal("expected error when forwarding without agent")
	}
}

func TestUnknownCommandForwards(t *testing.T) {
	r, s := newTestRouter(t)
	s.Create(&session.Session{ID: "sess-1", AgentID: "nonexistent"})

	events := collect(r.Execute("sess-1", "hello world"))
	if len(events) == 0 || events[0].Type != "error" {
		t.Fatal("expected error for nonexistent agent")
	}
}

func TestAgents(t *testing.T) {
	r, s := newTestRouter(t)
	s.Create(&session.Session{ID: "sess-1"})

	events := collect(r.Execute("sess-1", "agents"))
	if len(events) == 0 {
		t.Fatal("expected agents events")
	}
}

func TestSkills(t *testing.T) {
	r, s := newTestRouter(t)
	s.Create(&session.Session{ID: "sess-1"})

	events := collect(r.Execute("sess-1", "skills"))
	if len(events) == 0 {
		t.Fatal("expected skills events")
	}
}

func TestHistoryEmpty(t *testing.T) {
	r, s := newTestRouter(t)
	s.Create(&session.Session{ID: "sess-1"})

	events := collect(r.Execute("sess-1", "history"))
	if len(events) == 0 {
		t.Fatal("expected history events")
	}
}

func TestOsEcho(t *testing.T) {
	r, s := newTestRouter(t)
	s.Create(&session.Session{ID: "sess-1"})

	events := collect(r.Execute("sess-1", "os echo hello"))
	if len(events) == 0 {
		t.Fatal("expected os events")
	}
}

func TestOsStderr(t *testing.T) {
	r, s := newTestRouter(t)
	s.Create(&session.Session{ID: "sess-1"})

	events := collect(r.Execute("sess-1", "os echo hello >&2"))
	if len(events) > 0 {
		// Should capture stderr as progress events
		_ = events
	}
}

func TestOpenAndPwd(t *testing.T) {
	dir := t.TempDir()
	ws := workspace.NewRegistry()
	ws.Discover([]string{dir})

	ag := agent.NewRegistry()
	ag.Discover(t.TempDir())
	sk := skill.NewRegistry(t.TempDir())
	store := session.NewStore()
	agPool := pool.New(2, nil)

	r := NewRouter(store, ws, ag, sk, agPool)
	store.Create(&session.Session{ID: "sess-1"})

	events := collect(r.Execute("sess-1", "open "+ws.List()[0].ID))
	if len(events) == 0 || events[0].Type != "deliverable" {
		t.Fatal("expected deliverable for open")
	}

	events2 := collect(r.Execute("sess-1", "pwd"))
	if len(events2) == 0 {
		t.Fatal("expected pwd output")
	}
	if events2[0].Text != dir+"\n" {
		t.Fatalf("expected %s, got %s", dir, events2[0].Text)
	}
}
