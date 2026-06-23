package routing

import (
	"sync"
	"time"
)

type StickySelector struct {
	mu      sync.RWMutex
	ttl     time.Duration
	now     func() time.Time
	entries map[string]stickyEntry
}

type stickyEntry struct {
	AccountID int64
	ExpiresAt time.Time
}

func NewStickySelector(ttl time.Duration, now func() time.Time) *StickySelector {
	if ttl <= 0 {
		ttl = time.Minute
	}
	if now == nil {
		now = time.Now
	}
	return &StickySelector{
		ttl:     ttl,
		now:     now,
		entries: map[string]stickyEntry{},
	}
}

func (s *StickySelector) Apply(scope string, candidates []Candidate) []Candidate {
	ordered := ScoreCandidates(candidates)
	if len(ordered) <= 1 || s == nil {
		return ordered
	}

	entry, ok := s.entry(scope)
	if !ok {
		return ordered
	}
	for index, candidate := range ordered {
		if candidate.Account.ID != entry.AccountID {
			continue
		}
		if index == 0 {
			return ordered
		}
		remembered := ordered[index]
		copy(ordered[1:index+1], ordered[0:index])
		ordered[0] = remembered
		return ordered
	}
	return ordered
}

func (s *StickySelector) Remember(scope string, accountID int64) {
	if s == nil || scope == "" || accountID <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[scope] = stickyEntry{AccountID: accountID, ExpiresAt: s.now().UTC().Add(s.ttl)}
}

func (s *StickySelector) Current(scope string) (int64, bool) {
	if s == nil || scope == "" {
		return 0, false
	}
	entry, ok := s.entry(scope)
	if !ok {
		return 0, false
	}
	return entry.AccountID, true
}

func (s *StickySelector) Invalidate(scope string, accountID int64) {
	if s == nil || scope == "" || accountID <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[scope]
	if ok && entry.AccountID == accountID {
		delete(s.entries, scope)
	}
}

func (s *StickySelector) entry(scope string) (stickyEntry, bool) {
	s.mu.RLock()
	entry, ok := s.entries[scope]
	s.mu.RUnlock()
	if !ok {
		return stickyEntry{}, false
	}
	if !s.now().UTC().Before(entry.ExpiresAt) {
		s.mu.Lock()
		current, stillPresent := s.entries[scope]
		if stillPresent && current.AccountID == entry.AccountID && current.ExpiresAt.Equal(entry.ExpiresAt) {
			delete(s.entries, scope)
		}
		s.mu.Unlock()
		return stickyEntry{}, false
	}
	return entry, true
}
