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

const MergedSeparator = "\n\n"

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
