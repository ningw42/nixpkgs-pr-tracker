package event

import (
	"sync"
	"time"
)

type Type string

const (
	PRAdded        Type = "pr_added"
	PRRemoved      Type = "pr_removed"
	PRMerged       Type = "pr_merged"
	PRLandedBranch Type = "pr_landed_branch"
)

type Event struct {
	Type      Type
	PRNumber  int
	Title     string
	Author    string
	Branch    string
	Timestamp time.Time
}

type Handler func(Event)

type Bus struct {
	mu       sync.RWMutex
	handlers []Handler
}

func New() *Bus {
	return &Bus{}
}

func (b *Bus) Subscribe(h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, h)
}

func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, h := range b.handlers {
		h(e)
	}
}
