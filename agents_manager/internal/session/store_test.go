package session

import (
	"testing"
)

func TestStoreCreateAndGet(t *testing.T) {
	s := NewStore()

	sess := &Session{ID: "test-1", Transport: "repl", State: StateIdle}
	if err := s.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get("test-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != "test-1" {
		t.Fatalf("expected test-1, got %s", got.ID)
	}
	if got.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be set")
	}
}

func TestStoreUpdate(t *testing.T) {
	s := NewStore()

	s.Create(&Session{ID: "test-1"})
	sess, _ := s.Get("test-1")
	sess.WorkspaceID = "myapp"
	s.Update(sess)

	got, _ := s.Get("test-1")
	if got.WorkspaceID != "myapp" {
		t.Fatalf("expected workspace 'myapp', got %s", got.WorkspaceID)
	}
}

func TestStoreDelete(t *testing.T) {
	s := NewStore()

	s.Create(&Session{ID: "test-1"})
	s.Delete("test-1")

	if _, err := s.Get("test-1"); err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestStoreList(t *testing.T) {
	s := NewStore()

	s.Create(&Session{ID: "a"})
	s.Create(&Session{ID: "b"})

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(list))
	}
}

func TestStoreAddHistory(t *testing.T) {
	s := NewStore()

	s.Create(&Session{ID: "test-1"})
	s.AddHistory("test-1", Request{Text: "hello"})

	sess, _ := s.Get("test-1")
	if len(sess.History) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(sess.History))
	}
	if sess.History[0].Text != "hello" {
		t.Fatalf("expected 'hello', got %s", sess.History[0].Text)
	}
}

func TestStoreGetNotFound(t *testing.T) {
	s := NewStore()

	if _, err := s.Get("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestStoreUpdateNotFound(t *testing.T) {
	s := NewStore()

	err := s.Update(&Session{ID: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for updating nonexistent session")
	}
}
