package registry

import (
	"strings"
	"sync"
)

type Registry struct {
	mu       sync.Mutex
	injected map[string][]string
}

func New() *Registry {
	return &Registry{injected: map[string][]string{}}
}

func (r *Registry) SetInjected(runnerID string, docs ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range docs {
		if d != "" {
			r.injected[runnerID] = append(r.injected[runnerID], d)
		}
	}
}

// Consume reports whether text is the echo of a document this runner was
// handed, and if so retires that document (one-shot: a user retyping the same
// words later is still recorded as their own).
//
// Containment, not equality: what actually reaches the CLI as a positional
// prompt is the DESCRIPTOR's own template rendering of the doc Go computed —
// claude's resume pointer wraps it in <system-reminder> tags Go never
// mentions, and codex's does not. Go registers the bare doc it composed; the
// wire text a hook echoes back is whatever the provider's own template made
// of it. An equality check here would treat every such wrapping as a foreign
// message and start recording the injected handoff as the user's own turn.
func (r *Registry) Consume(runnerID, text string) bool {
	if text == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	docs := r.injected[runnerID]
	for i, d := range docs {
		if !strings.Contains(text, d) {
			continue
		}
		r.injected[runnerID] = append(docs[:i:i], docs[i+1:]...)
		return true
	}
	return false
}

func (r *Registry) Forget(runnerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.injected, runnerID)
}
