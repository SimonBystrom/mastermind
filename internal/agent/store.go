package agent

import (
	"fmt"
	"sync"
	"sync/atomic"
)

type Store struct {
	mu     sync.RWMutex
	agents map[string]*Agent
	nextID atomic.Int64
	dirty  atomic.Bool
}

func NewStore() *Store {
	return &Store{
		agents: make(map[string]*Agent),
	}
}

func (s *Store) Add(a *Agent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.ID == "" {
		id := s.nextID.Add(1)
		a.ID = fmt.Sprintf("a%d", id)
	}
	s.agents[a.ID] = a
	s.dirty.Store(true)
}

// MarkDirty marks the store as having unsaved changes.
func (s *Store) MarkDirty() {
	s.dirty.Store(true)
}

// IsDirty returns true if the store has unsaved changes.
func (s *Store) IsDirty() bool {
	return s.dirty.Load()
}

// ClearDirty resets the dirty flag.
func (s *Store) ClearDirty() {
	s.dirty.Store(false)
}

func (s *Store) Get(id string) (*Agent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.agents[id]
	return a, ok
}

func (s *Store) All() []*Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Agent, 0, len(s.agents))
	for _, a := range s.agents {
		result = append(result, a)
	}
	return result
}

func (s *Store) UpdateStatus(id string, status Status) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.agents[id]
	if !ok {
		return false
	}
	a.SetStatus(status)
	s.dirty.Store(true)
	return true
}

func (s *Store) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.agents, id)
	s.dirty.Store(true)
}
