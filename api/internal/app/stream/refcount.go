package stream

import "github.com/char2cs/crowbar/api/internal/domain/stream"

// Resource is daemon work that runs while any client holds a scope naming it.
type Resource interface {
	Start(key string)
	Stop(key string)
}

type held struct {
	res Resource
	key string
}

// refcount counts holds per (resource, key) across every scope kind listing
// the resource, so one watcher under git: and files: starts once. The hub's
// lock guards it.
type refcount struct {
	byKind map[stream.ScopeKind][]Resource
	counts map[held]int
}

func newRefcount(byKind map[stream.ScopeKind][]Resource) *refcount {
	return &refcount{byKind: byKind, counts: map[held]int{}}
}

func (r *refcount) hold(scope stream.Scope) {
	for _, res := range r.byKind[scope.Kind] {
		k := held{res: res, key: scope.Key}
		r.counts[k]++
		if r.counts[k] == 1 {
			res.Start(scope.Key)
		}
	}
}

func (r *refcount) release(scope stream.Scope) {
	for _, res := range r.byKind[scope.Kind] {
		k := held{res: res, key: scope.Key}
		r.counts[k]--
		if r.counts[k] == 0 {
			delete(r.counts, k)
			res.Stop(scope.Key)
		}
	}
}
