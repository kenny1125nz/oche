package session

import (
	"fmt"
	"sync"
	"time"
)

type Storer interface {
	Create(sess *Session) error
	Get(id string) (*Session, error)
	Update(sess *Session) error
	Delete(id string) error
	List() []*Session
	AddHistory(sessionID string, req Request) error
}

type Store struct {
	mu     sync.RWMutex
	memory map[string]*Session
}

func NewStore() *Store {
	return &Store{
		memory: make(map[string]*Session),
	}
}

func (s *Store) Create(sess *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess.CreatedAt = time.Now()
	sess.UpdatedAt = time.Now()
	s.memory[sess.ID] = sess
	return nil
}

func (s *Store) Get(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, ok := s.memory[id]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	return sess, nil
}

func (s *Store) Update(sess *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.memory[sess.ID]; !ok {
		return fmt.Errorf("session not found: %s", sess.ID)
	}

	sess.UpdatedAt = time.Now()
	s.memory[sess.ID] = sess
	return nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.memory, id)
	return nil
}

func (s *Store) List() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Session, 0, len(s.memory))
	for _, sess := range s.memory {
		result = append(result, sess)
	}
	return result
}

func (s *Store) AddHistory(sessionID string, req Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.memory[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	sess.History = append(sess.History, req)
	sess.UpdatedAt = time.Now()
	return nil
}
