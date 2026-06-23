package api

import (
	"sync"

	"github.com/gcssloop/codex-router/backend/internal/usage"
)

type AsyncUsageStoreOptions struct {
	QueueSize int
}

type AsyncUsageStore struct {
	base  usageEventStore
	tasks chan func()
	done  chan struct{}

	cacheMu sync.RWMutex
	cache   map[int64]usage.Snapshot

	wg sync.WaitGroup
}

func NewAsyncUsageStore(base usageEventStore, opts AsyncUsageStoreOptions) *AsyncUsageStore {
	queueSize := opts.QueueSize
	if queueSize <= 0 {
		queueSize = 4096
	}
	store := &AsyncUsageStore{
		base:  base,
		tasks: make(chan func(), queueSize),
		done:  make(chan struct{}),
		cache: map[int64]usage.Snapshot{},
	}
	store.wg.Add(1)
	go store.run()
	return store
}

func (s *AsyncUsageStore) GetLatest(accountID int64) (usage.Snapshot, error) {
	s.cacheMu.RLock()
	snapshot, ok := s.cache[accountID]
	s.cacheMu.RUnlock()
	if ok {
		return snapshot, nil
	}
	snapshot, err := s.base.GetLatest(accountID)
	if err != nil {
		return usage.Snapshot{}, err
	}
	s.rememberSnapshot(snapshot)
	return snapshot, nil
}

func (s *AsyncUsageStore) Save(snapshot usage.Snapshot) error {
	s.rememberSnapshot(snapshot)
	s.enqueue(func() {
		_ = s.base.Save(snapshot)
	})
	return nil
}

func (s *AsyncUsageStore) SaveEvent(event usage.Event) error {
	s.enqueue(func() {
		_ = s.base.SaveEvent(event)
	})
	return nil
}

func (s *AsyncUsageStore) EnqueueUsageEvent(task usageEventTask) bool {
	return s.enqueue(func() {
		if saved, ok := persistUsageEventSync(s.base, task); ok {
			s.rememberSnapshot(saved)
		}
	})
}

func (s *AsyncUsageStore) Drain() {
	done := make(chan struct{})
	if !s.enqueue(func() {
		close(done)
	}) {
		return
	}
	<-done
}

func (s *AsyncUsageStore) Close() {
	close(s.done)
	s.wg.Wait()
}

func (s *AsyncUsageStore) enqueue(task func()) bool {
	select {
	case <-s.done:
		return false
	case s.tasks <- task:
		return true
	default:
		return false
	}
}

func (s *AsyncUsageStore) run() {
	defer s.wg.Done()
	for {
		select {
		case <-s.done:
			for {
				select {
				case task := <-s.tasks:
					task()
				default:
					return
				}
			}
		case task := <-s.tasks:
			task()
		}
	}
}

func (s *AsyncUsageStore) rememberSnapshot(snapshot usage.Snapshot) {
	if snapshot.AccountID == 0 {
		return
	}
	s.cacheMu.Lock()
	s.cache[snapshot.AccountID] = snapshot
	s.cacheMu.Unlock()
}
