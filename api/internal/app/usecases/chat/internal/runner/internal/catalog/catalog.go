package catalog

import (
	"context"
	"sync"
)

const maxProcesses = 4

type run struct {
	id     uint64
	cancel context.CancelFunc
}

type Runs struct {
	mu           sync.Mutex
	nextID       uint64
	runs         map[string]run
	processSlots chan struct{}
}

func New() *Runs {
	return &Runs{
		runs:         map[string]run{},
		processSlots: make(chan struct{}, maxProcesses),
	}
}

func (r *Runs) Start(parent context.Context, chatID string) (context.Context, func()) {
	r.mu.Lock()
	if old, ok := r.runs[chatID]; ok {
		old.cancel()
	}
	r.nextID++
	id := r.nextID
	ctx, cancel := context.WithCancel(parent)
	r.runs[chatID] = run{id: id, cancel: cancel}
	r.mu.Unlock()
	return ctx, func() {
		cancel()
		r.mu.Lock()
		if current, ok := r.runs[chatID]; ok && current.id == id {
			delete(r.runs, chatID)
		}
		r.mu.Unlock()
	}
}

func (r *Runs) AcquireProcess(ctx context.Context) (func(), error) {
	select {
	case r.processSlots <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-r.processSlots }) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
