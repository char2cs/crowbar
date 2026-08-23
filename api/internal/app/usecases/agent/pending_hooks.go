package agent

import (
	"fmt"
	"sync"
)

const (
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
