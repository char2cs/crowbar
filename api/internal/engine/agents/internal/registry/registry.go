package registry

import "sync"

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

func (r *Registry) Forget(runnerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.injected, runnerID)
}
