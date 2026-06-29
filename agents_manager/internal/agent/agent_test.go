package agent

import (
	"os"
	"testing"
)

func TestDiscoverExecutableFiles(t *testing.T) {
	home := t.TempDir()
	agentsDir := home + "/agents"
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(agentsDir+"/my-agent", []byte("#!/bin/sh\necho hello"), 0755)
	os.WriteFile(agentsDir+"/not-executable", []byte("data"), 0644)

	r := NewRegistry()
	if err := r.Discover(home); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if r.Len() != 1 {
		t.Fatalf("expected 1 agent, got %d", r.Len())
	}

	reg, err := r.Get("my-agent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if reg.ID != "my-agent" {
		t.Fatalf("expected 'my-agent', got %s", reg.ID)
	}
}

func TestDiscoverAgentDirectories(t *testing.T) {
	home := t.TempDir()
	agentsDir := home + "/agents"
	os.MkdirAll(agentsDir+"/claude-code/bin", 0755)
	os.WriteFile(agentsDir+"/claude-code/bin/claude-code", []byte("#!/bin/sh\necho claude"), 0755)

	r := NewRegistry()
	if err := r.Discover(home); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if r.Len() != 1 {
		t.Fatalf("expected 1 agent, got %d", r.Len())
	}

	reg, err := r.Get("claude-code")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if reg.Binary != agentsDir+"/claude-code/bin/claude-code" {
		t.Fatalf("expected binary at bin/claude-code, got %s", reg.Binary)
	}
}

func TestDiscoverMissingDir(t *testing.T) {
	r := NewRegistry()
	if err := r.Discover("/nonexistent"); err != nil {
		t.Fatalf("Discover should not error on missing dir: %v", err)
	}
	if r.Len() != 0 {
		t.Fatalf("expected 0, got %d", r.Len())
	}
}

func TestGetNotFound(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Get("nope"); err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

func TestDiscoverSystem(t *testing.T) {
	r := NewRegistry()
	r.DiscoverSystem()
	// In a test environment, no known system agents should be found.
	// This just verifies it doesn't panic or error.
}

func TestDiscoverSystemDoesNotOverrideUserAgent(t *testing.T) {
	home := t.TempDir()
	agentsDir := home + "/agents"
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(agentsDir+"/claude", []byte("#!/bin/sh\necho fake-claude"), 0755)

	r := NewRegistry()
	r.Discover(home)
	r.DiscoverSystem()

	reg, err := r.Get("claude")
	if err != nil {
		t.Fatalf("Get claude: %v", err)
	}
	if reg.System {
		t.Fatal("user agent should not be marked as system")
	}
	if reg.Path != agentsDir {
		t.Fatalf("expected user agent path, got %s", reg.Path)
	}
}

func TestList(t *testing.T) {
	home := t.TempDir()
	agentsDir := home + "/agents"
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(agentsDir+"/zzz-agent", []byte("#!/bin/sh\n"), 0755)
	os.WriteFile(agentsDir+"/aaa-agent", []byte("#!/bin/sh\n"), 0755)

	r := NewRegistry()
	r.Discover(home)

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
	if list[0].ID != "aaa-agent" || list[1].ID != "zzz-agent" {
		t.Fatalf("expected sorted order, got %s %s", list[0].ID, list[1].ID)
	}
}
