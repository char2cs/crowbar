// Package pending buffers the hooks a vendor CLI fires before Crowbar has
// persisted the runner row those hooks belong to.
package pending

import (
	"fmt"
	"sync"
)

const (
	maxHooks     = 64
	maxHookBytes = 32 << 20
)

// Hook is one buffered hook: everything the ingest path needs to replay it once
// the runner row exists. The delivery fields are empty for the un-journalled
// (local, non-relayed) ingress.
type Hook struct {
	Provider       string
	CanonicalEvent string
	RawPayload     []byte
	DeliveryID     string
	DeliveryDir    string
	DeliveryHash   string
}

type entry struct {
	hooks  []Hook
	bytes  int
	exited bool
}

// Hooks is the fork-before-runner-persistence barrier. A spawn Registers the
// runner id before forking the CLI, every hook that arrives in the window is
// Enqueued instead of dropped, and Finish replays them in arrival order once the
// row is durable.
type Hooks struct {
	mu      sync.Mutex
	entries map[string]*entry
}

// New returns an empty barrier.
func New() *Hooks {
	return &Hooks{entries: map[string]*entry{}}
}

// Register opens the buffer for runnerID. Registering twice is a programming
// error, not a race to tolerate: it means two spawns claimed one runner id.
func (p *Hooks) Register(runnerID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.entries[runnerID]; exists {
		return fmt.Errorf("agent: pending hooks: runner %s is already registered", runnerID)
	}
	p.entries[runnerID] = &entry{}
	return nil
}

// Enqueue buffers an un-journalled hook. handled is false when the runner is not
// in its startup window, which tells the caller to ingest normally.
func (p *Hooks) Enqueue(
	runnerID, provider, canonicalEvent string,
	rawPayload []byte,
) (handled bool, err error) {
	return p.enqueue(runnerID, Hook{
		Provider:       provider,
		CanonicalEvent: canonicalEvent,
		RawPayload:     rawPayload,
	})
}

// EnqueueDelivery buffers a journalled (relayed) hook, deduplicating on the
// delivery id so a relay retry inside the startup window is buffered once.
func (p *Hooks) EnqueueDelivery(
	runnerID, provider, canonicalEvent string,
	rawPayload []byte,
	deliveryID, deliveryDir, deliveryHash string,
) (handled bool, err error) {
	return p.enqueue(runnerID, Hook{
		Provider:       provider,
		CanonicalEvent: canonicalEvent,
		RawPayload:     rawPayload,
		DeliveryID:     deliveryID,
		DeliveryDir:    deliveryDir,
		DeliveryHash:   deliveryHash,
	})
}

func (p *Hooks) enqueue(
	runnerID string,
	hook Hook,
) (handled bool, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, exists := p.entries[runnerID]
	if !exists {
		return false, nil
	}
	if hook.DeliveryID != "" {
		for _, queued := range e.hooks {
			if queued.DeliveryID == hook.DeliveryID {
				return true, nil
			}
		}
	}
	if len(e.hooks) >= maxHooks || e.bytes+len(hook.RawPayload) > maxHookBytes {
		return true, fmt.Errorf("agent: pending hooks: startup buffer limit exceeded")
	}
	hook.RawPayload = append([]byte(nil), hook.RawPayload...)
	e.hooks = append(e.hooks, hook)
	e.bytes += len(hook.RawPayload)
	return true, nil
}

// MarkExited records that the CLI died inside its own startup window. It reports
// whether the runner was still registered, so a caller can tell a death from a
// spawn that had already finished.
func (p *Hooks) MarkExited(runnerID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, exists := p.entries[runnerID]
	if !exists {
		return false
	}
	e.exited = true
	return true
}

// Discard drops the buffer without replaying it: the spawn failed and the runner
// row will never exist.
func (p *Hooks) Discard(runnerID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.entries, runnerID)
}

// Finish replays every buffered hook in arrival order and closes the window. It
// loops because handle may itself cause more hooks to arrive; the buffer is only
// removed once a pass finds it empty, which is what makes the handoff from
// buffered to direct ingest atomic. It reports whether the CLI exited while
// buffered.
func (p *Hooks) Finish(
	runnerID string,
	handle func(Hook),
) (exited bool) {
	for {
		p.mu.Lock()
		e, exists := p.entries[runnerID]
		if !exists {
			p.mu.Unlock()
			return false
		}
		if len(e.hooks) == 0 {
			exited = e.exited
			delete(p.entries, runnerID)
			p.mu.Unlock()
			return exited
		}
		batch := append([]Hook(nil), e.hooks...)
		e.hooks = nil
		e.bytes = 0
		p.mu.Unlock()

		for _, hook := range batch {
			handle(hook)
		}
	}
}
