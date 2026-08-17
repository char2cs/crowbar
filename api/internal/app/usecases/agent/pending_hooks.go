package agent

import (
	"fmt"
	"sync"
)

const (
	// A provider normally emits at most session_start, user_prompt and turn_stop
	// during startup. These limits keep the pre-persistence race buffer bounded
	// even if a local hook source is broken or hostile; the CLI relay itself caps
	// each raw payload at 8 MiB.
	maxPendingRunnerHooks     = 64
	maxPendingRunnerHookBytes = 32 << 20
)

type pendingRunnerHook struct {
	provider       string
	canonicalEvent string
	rawPayload     []byte
	deliveryID     string
	deliveryDir    string
	deliveryHash   string
}

type pendingRunnerHookEntry struct {
	hooks  []pendingRunnerHook
	bytes  int
	exited bool
}

// pendingRunnerHooks closes the unavoidable fork-before-persistence gap. A
// provider can synchronously fire its startup and positional-prompt hooks as
// soon as the PTY starts, while the runner aggregate cannot be recorded until
// CreateCommand returns the terminal session id. Hooks for a pre-registered
// runner are buffered in arrival order and replayed once its durable placement
// exists.
//
// An entry remains installed while replay is happening. Hooks arriving during
// replay therefore join the next batch instead of overtaking an earlier hook on
// the normal ingestion path. The same entry records an early PTY exit so a
// process that dies before persistence cannot leave a live runner row behind.
type pendingRunnerHooks struct {
	mu      sync.Mutex
	entries map[string]*pendingRunnerHookEntry
}

func newPendingRunnerHooks() *pendingRunnerHooks {
	return &pendingRunnerHooks{entries: map[string]*pendingRunnerHookEntry{}}
}

func (p *pendingRunnerHooks) register(runnerID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.entries[runnerID]; exists {
		return fmt.Errorf("agent: pending hooks: runner %s is already registered", runnerID)
	}
	p.entries[runnerID] = &pendingRunnerHookEntry{}
	return nil
}

// enqueue returns handled=true when runnerID is currently in its startup gap.
// The payload is copied because the HTTP request buffer belongs to the caller.
func (p *pendingRunnerHooks) enqueue(
	runnerID, provider, canonicalEvent string,
	rawPayload []byte,
) (handled bool, err error) {
	return p.enqueueHook(runnerID, pendingRunnerHook{
		provider:       provider,
		canonicalEvent: canonicalEvent,
		rawPayload:     rawPayload,
	})
}

func (p *pendingRunnerHooks) enqueueDelivery(
	runnerID, provider, canonicalEvent string,
	rawPayload []byte,
	deliveryID, deliveryDir, deliveryHash string,
) (handled bool, err error) {
	return p.enqueueHook(runnerID, pendingRunnerHook{
		provider:       provider,
		canonicalEvent: canonicalEvent,
		rawPayload:     rawPayload,
		deliveryID:     deliveryID,
		deliveryDir:    deliveryDir,
		deliveryHash:   deliveryHash,
	})
}

func (p *pendingRunnerHooks) enqueueHook(
	runnerID string,
	hook pendingRunnerHook,
) (handled bool, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, exists := p.entries[runnerID]
	if !exists {
		return false, nil
	}
	if hook.deliveryID != "" {
		for _, queued := range entry.hooks {
			if queued.deliveryID == hook.deliveryID {
				return true, nil
			}
		}
	}
	if len(entry.hooks) >= maxPendingRunnerHooks || entry.bytes+len(hook.rawPayload) > maxPendingRunnerHookBytes {
		return true, fmt.Errorf("agent: pending hooks: startup buffer limit exceeded")
	}
	hook.rawPayload = append([]byte(nil), hook.rawPayload...)
	entry.hooks = append(entry.hooks, hook)
	entry.bytes += len(hook.rawPayload)
	return true, nil
}

// markExited returns true when the runner is still inside the startup barrier.
// Its caller then defers aggregate reconciliation until finish has persisted and
// replayed the buffered hooks.
func (p *pendingRunnerHooks) markExited(runnerID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, exists := p.entries[runnerID]
	if !exists {
		return false
	}
	entry.exited = true
	return true
}

func (p *pendingRunnerHooks) discard(runnerID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.entries, runnerID)
}

// finish drains every batch in arrival order. The entry is deleted atomically
// with observing an empty queue, so a hook either joins a replay batch or sees
// no barrier and takes the now-persisted normal path; there is no drop window.
func (p *pendingRunnerHooks) finish(
	runnerID string,
	handle func(pendingRunnerHook),
) (exited bool) {
	for {
		p.mu.Lock()
		entry, exists := p.entries[runnerID]
		if !exists {
			p.mu.Unlock()
			return false
		}
		if len(entry.hooks) == 0 {
			exited = entry.exited
			delete(p.entries, runnerID)
			p.mu.Unlock()
			return exited
		}
		batch := append([]pendingRunnerHook(nil), entry.hooks...)
		entry.hooks = nil
		entry.bytes = 0
		p.mu.Unlock()

		for _, hook := range batch {
			handle(hook)
		}
	}
}
