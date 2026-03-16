package api

import (
	"context"
	"sync"
)

const AccountRoutingStateChangedTopic = "accounts-routing-changed"

type StateEventBus struct {
	mu          sync.Mutex
	subscribers map[chan string]struct{}
}

func NewStateEventBus() *StateEventBus {
	return &StateEventBus{
		subscribers: make(map[chan string]struct{}),
	}
}

func (b *StateEventBus) Subscribe(ctx context.Context) <-chan string {
	ch := make(chan string, 4)
	if b == nil {
		close(ch)
		return ch
	}

	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	go func() {
		<-ctx.Done()
		b.mu.Lock()
		if _, ok := b.subscribers[ch]; ok {
			delete(b.subscribers, ch)
			close(ch)
		}
		b.mu.Unlock()
	}()

	return ch
}

func (b *StateEventBus) Publish(topic string) {
	if b == nil || topic == "" {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subscribers {
		select {
		case ch <- topic:
		default:
		}
	}
}
