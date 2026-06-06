package server

import (
	"sort"
	"sync"
)

// OpenDocs is the content-free set of currently open document URIs for a
// language server. Add/Remove/List are safe for concurrent use. The server
// replays didOpen for every tracked URI on respawn (10 §3).
type OpenDocs struct {
	mu   sync.Mutex
	uris map[string]struct{}
}

// NewOpenDocs returns an empty OpenDocs set.
func NewOpenDocs() *OpenDocs {
	return &OpenDocs{uris: make(map[string]struct{})}
}

// Add records uri as open. Adding an already-tracked uri is a no-op.
func (d *OpenDocs) Add(
	uri string,
) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.uris[uri] = struct{}{}
}

// Remove drops uri from the open set. Removing an absent uri is a no-op.
func (d *OpenDocs) Remove(
	uri string,
) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.uris, uri)
}

// List returns the tracked URIs sorted lexicographically for deterministic
// replay order.
func (d *OpenDocs) List() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	out := make([]string, 0, len(d.uris))
	for uri := range d.uris {
		out = append(out, uri)
	}
	sort.Strings(out)
	return out
}
