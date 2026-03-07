package policy

import (
	"errors"
	"sync"
)

type Repository interface {
	Save(def Definition) error
	Get(name string) (Definition, error)
}

type MemoryRepository struct {
	mu      sync.RWMutex
	entries map[string]Definition
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{entries: make(map[string]Definition)}
}

func (r *MemoryRepository) Save(def Definition) error {
	if def.Name == "" {
		return errors.New("policy name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[def.Name] = def
	return nil
}

func (r *MemoryRepository) Get(name string) (Definition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.entries[name]
	if !ok {
		return Definition{}, errors.New("policy not found")
	}
	return def, nil
}
