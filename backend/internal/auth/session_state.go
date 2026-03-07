package auth

import (
	"errors"
	"sync"
	"time"
)

type StateStore struct {
	ttl    time.Duration
	mu     sync.Mutex
	states map[string]time.Time
}

func NewStateStore(ttl time.Duration) *StateStore {
	return &StateStore{
		ttl:    ttl,
		states: make(map[string]time.Time),
	}
}

func (s *StateStore) New() (string, error) {
	state, err := randomState()
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state] = time.Now().UTC().Add(s.ttl)
	return state, nil
}

func (s *StateStore) Validate(state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	expiresAt, ok := s.states[state]
	if !ok {
		return errors.New("state not found")
	}
	delete(s.states, state)

	if time.Now().UTC().After(expiresAt) {
		return errors.New("state expired")
	}

	return nil
}
