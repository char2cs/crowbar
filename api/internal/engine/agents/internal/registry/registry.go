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

// MergedSeparator joins the rendered context step to the message step in one
// positional argv token. Shared so the merge and consume sides cannot drift.
const MergedSeparator = "\n\n"

// ConsumePrefix retires (one-shot) the first injected doc that text CONTAINS (the
// descriptor's template may wrap it) and returns the user's prompt after
// MergedSeparator, or "" for a bare echo, so injected context is never their turn.
func (r *Registry) ConsumePrefix(runnerID, text string) (remainder string, found bool) {
	if text == "" {
		return text, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	docs := r.injected[runnerID]
	for i, d := range docs {
		idx := strings.Index(text, d)
		if idx < 0 {
			continue
		}
		r.injected[runnerID] = append(docs[:i:i], docs[i+1:]...)
		// Search for the separator only AFTER the doc: the doc itself contains
		// blank lines, and any template wrapping sits between its end and the
		// separator. No separator means a bare echo.
		after := text[idx+len(d):]
		if sep := strings.Index(after, MergedSeparator); sep >= 0 {
			return strings.TrimSpace(after[sep+len(MergedSeparator):]), true
		}
		return "", true
	}
	return text, false
}

func (r *Registry) Forget(runnerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.injected, runnerID)
}
