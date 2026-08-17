// Package registry remembers what Crowbar injected into a live CLI, so the CLI
// echoing it back can be recognised rather than recorded.
package registry

import "sync"

// Registry holds genuinely ephemeral, per-spawn state: the text Crowbar injected
// into a CLI at spawn, kept only long enough to recognise that CLI echoing it
// back. It has no durable counterpart it could drift from, and no meaning at all
// once the process is gone.
//
// This is deliberately all that is left of an older registry that also shadowed
// segment→chat and session→chat placement. Those were an in-memory copy of state
// that is also durable, mutated BEFORE the aggregate commands ran, so a failed
// command left the registry believing a runner had moved — and the orphaned CLI's
// turn hooks were then routed into a chat it had left. Placement is read from the
// runner aggregate now: one authority, no shadow.
type Registry struct {
	mu       sync.Mutex
	injected map[string][]string // runner id -> everything Crowbar injected at spawn
}

func New() *Registry {
	return &Registry{injected: map[string][]string{}}
}

// SetInjected records everything Crowbar injected into a runner at spawn — the
// context document AND the pointer message a provider reachable only through a
// user message receives — so Consume can recognise whichever comes back.
func (r *Registry) SetInjected(runnerID string, docs ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range docs {
		if d != "" {
			r.injected[runnerID] = append(r.injected[runnerID], d)
		}
	}
}

// Consume reports whether text is the very document Crowbar injected into this
// runner at spawn — i.e. the CLI is echoing our own handoff back through its
// user-prompt hook, not relaying something the user typed.
//
// Provider-agnostic on purpose: a provider whose only resume channel is a user
// message fires user_prompt for the injected document, while one with a silent
// channel never does, and the registry does not need to know which is which.
//
// Recording that echo as a turn is what made handoffs NEST — the blob became a
// "user" turn, and the next handoff embedded it inside itself. The match is
// one-shot, so a user who genuinely retypes the same text later is still
// recorded.
func (r *Registry) Consume(runnerID, text string) bool {
	if text == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	docs := r.injected[runnerID]
	for i, d := range docs {
		if d != text {
			continue
		}
		r.injected[runnerID] = append(docs[:i:i], docs[i+1:]...)
		return true
	}
	return false
}

// Forget drops a dead runner's entries. The echo guard exists to recognise a
// document coming back from a LIVE process, so once the process is gone the entry
// has no meaning and would otherwise accumulate one handoff-sized string per
// spawn for the life of the daemon.
func (r *Registry) Forget(runnerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.injected, runnerID)
}
