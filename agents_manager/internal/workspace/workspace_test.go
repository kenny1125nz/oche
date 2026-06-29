package workspace

import (
	"os"
	"testing"
)

func TestDiscoverExactPaths(t *testing.T) {
	dirA := t.TempDir() + "/project-alpha"
	dirB := t.TempDir() + "/project-beta"
	os.MkdirAll(dirA, 0755)
	os.MkdirAll(dirB, 0755)

	r := NewRegistry()
	if err := r.Discover([]string{dirA, dirB}); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if r.Len() != 2 {
		t.Fatalf("expected 2 workspaces, got %d", r.Len())
	}

	ws, err := r.Get("project-alpha")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ws.TargetDir != dirA {
		t.Fatalf("expected target_dir %s, got %s", dirA, ws.TargetDir)
	}
}

func TestDiscoverGlobPaths(t *testing.T) {
	base := t.TempDir()
	os.MkdirAll(base+"/sub1", 0755)
	os.MkdirAll(base+"/sub2", 0755)
	os.MkdirAll(base+"/sub3", 0755)

	r := NewRegistry()
	if err := r.Discover([]string{base + "/*"}); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if r.Len() != 3 {
		t.Fatalf("expected 3 workspaces, got %d", r.Len())
	}

	for _, name := range []string{"sub1", "sub2", "sub3"} {
		if _, err := r.Get(name); err != nil {
			t.Fatalf("expected workspace %s: %v", name, err)
		}
	}
}

func TestDiscoverMissingPath(t *testing.T) {
	r := NewRegistry()
	if err := r.Discover([]string{"/nonexistent/path"}); err != nil {
		t.Fatalf("Discover should not error on missing path: %v", err)
	}
	if r.Len() != 0 {
		t.Fatalf("expected 0 workspaces, got %d", r.Len())
	}
}

func TestGetNotFound(t *testing.T) {
	r := NewRegistry()
	if _, err := r.Get("nope"); err == nil {
		t.Fatal("expected error for nonexistent workspace")
	}
}

func TestList(t *testing.T) {
	dirA := t.TempDir() + "/zzz-last"
	dirB := t.TempDir() + "/aaa-first"
	os.MkdirAll(dirA, 0755)
	os.MkdirAll(dirB, 0755)

	r := NewRegistry()
	r.Discover([]string{dirA, dirB})

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
	if list[0].ID != "aaa-first" || list[1].ID != "zzz-last" {
		t.Fatalf("expected sorted order, got %s %s", list[0].ID, list[1].ID)
	}
}
