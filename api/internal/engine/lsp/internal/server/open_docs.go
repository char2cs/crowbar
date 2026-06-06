package server

import (
	"sort"
	"sync"
)

type openDocs struct {
	mu   sync.Mutex
	uris map[string]struct{}
}

func newOpenDocs() *openDocs {
	return &openDocs{uris: make(map[string]struct{})}
}

// Add records uri as open. Adding an already-tracked uri is a no-op.
func (d *openDocs) Add(
	uri string,
) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.uris[uri] = struct{}{}
}

// Remove drops uri from the open set. Removing an absent uri is a no-op.
func (d *openDocs) Remove(
	uri string,
) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.uris, uri)
}

// List returns the tracked URIs sorted lexicographically for deterministic
// replay order.
func (d *openDocs) List() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	out := make([]string, 0, len(d.uris))
	for uri := range d.uris {
		out = append(out, uri)
	}
	sort.Strings(out)
	return out
}
