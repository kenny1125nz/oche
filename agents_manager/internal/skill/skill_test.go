package skill

import (
	"os"
	"testing"
)

func TestGetGlobal(t *testing.T) {
	home := t.TempDir()
	skillsDir := home + "/skills"
	os.MkdirAll(skillsDir, 0755)
	os.WriteFile(skillsDir+"/lint", []byte("#!/bin/sh\necho lint"), 0755)
	os.WriteFile(skillsDir+"/test", []byte("description"), 0755)

	r := NewRegistry(home)
	skills := r.GetGlobal()

	if len(skills) != 2 {
		t.Fatalf("expected 2 global skills, got %d", len(skills))
	}
}

func TestGetGlobalWithDescription(t *testing.T) {
	home := t.TempDir()
	skillsDir := home + "/skills"
	os.MkdirAll(skillsDir, 0755)
	os.WriteFile(skillsDir+"/lint", []byte("#!/bin/sh\necho lint"), 0755)
	os.WriteFile(skillsDir+"/lint.desc", []byte("Run linter on source files"), 0644)

	r := NewRegistry(home)
	skills := r.GetGlobal()

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Description != "Run linter on source files" {
		t.Fatalf("expected desc from .desc file, got %q", skills[0].Description)
	}
}

func TestGetWorkspaceSkills(t *testing.T) {
	wsPath := t.TempDir()
	os.MkdirAll(wsPath+"/.agents/skills", 0755)
	os.WriteFile(wsPath+"/.agents/skills/deploy", []byte("#!/bin/sh\n"), 0755)

	r := NewRegistry(t.TempDir())
	skills := r.GetWorkspaceSkills(wsPath)

	if len(skills) != 1 {
		t.Fatalf("expected 1 workspace skill, got %d", len(skills))
	}
	if skills[0].Name != "deploy" {
		t.Fatalf("expected 'deploy', got %s", skills[0].Name)
	}
}

func TestGetAgentSkills(t *testing.T) {
	agentDir := t.TempDir()
	os.MkdirAll(agentDir+"/skills", 0755)
	os.WriteFile(agentDir+"/skills/review", []byte("#!/bin/sh\n"), 0755)

	r := NewRegistry(t.TempDir())
	skills := r.GetAgentSkills(agentDir)

	if len(skills) != 1 {
		t.Fatalf("expected 1 agent skill, got %d", len(skills))
	}
}

func TestGetAllMergeOrder(t *testing.T) {
	home := t.TempDir()
	wsPath := t.TempDir()
	agentDir := t.TempDir()

	// Global skill
	os.MkdirAll(home+"/skills", 0755)
	os.WriteFile(home+"/skills/global-skill", []byte("#!/bin/sh\n"), 0755)

	// Workspace skill
	os.MkdirAll(wsPath+"/.agents/skills", 0755)
	os.WriteFile(wsPath+"/.agents/skills/ws-skill", []byte("#!/bin/sh\n"), 0755)

	// Agent skill
	os.MkdirAll(agentDir+"/skills", 0755)
	os.WriteFile(agentDir+"/skills/agent-skill", []byte("#!/bin/sh\n"), 0755)

	r := NewRegistry(home)
	skills := r.GetAll(wsPath, agentDir)

	// Agent > workspace > global
	if len(skills) != 3 {
		t.Fatalf("expected 3 skills, got %d", len(skills))
	}
	if skills[0].Name != "agent-skill" {
		t.Fatalf("expected first to be agent-skill, got %s", skills[0].Name)
	}
	if skills[1].Name != "ws-skill" {
		t.Fatalf("expected second to be ws-skill, got %s", skills[1].Name)
	}
	if skills[2].Name != "global-skill" {
		t.Fatalf("expected third to be global-skill, got %s", skills[2].Name)
	}
}

func TestGetAllDeduplicate(t *testing.T) {
	home := t.TempDir()
	wsPath := t.TempDir()
	agentDir := t.TempDir()

	os.MkdirAll(home+"/skills", 0755)
	os.WriteFile(home+"/skills/my-skill", []byte("#!/bin/sh\n"), 0755)

	os.MkdirAll(wsPath+"/.agents/skills", 0755)
	os.WriteFile(wsPath+"/.agents/skills/my-skill", []byte("#!/bin/sh\n"), 0755)

	r := NewRegistry(home)
	skills := r.GetAll(wsPath, agentDir)

	// Only one "my-skill" because agent doesn't have it, workspace overrides global
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill (deduped), got %d", len(skills))
	}
}

func TestGetAllEmptyPaths(t *testing.T) {
	r := NewRegistry(t.TempDir())
	skills := r.GetAll("", "")

	if len(skills) != 0 {
		t.Fatalf("expected 0 skills with empty paths, got %d", len(skills))
	}
}

func TestGetGlobalNoDir(t *testing.T) {
	r := NewRegistry(t.TempDir())
	skills := r.GetGlobal()
	if len(skills) != 0 {
		t.Fatalf("expected 0 skills with no dir, got %d", len(skills))
	}
}
