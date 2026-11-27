package server

import (
	"sort"
	"sync"
)

type OpenDocs struct {
	mu   sync.Mutex
	uris map[string]struct{}
}

func NewOpenDocs() *OpenDocs {
	return &OpenDocs{uris: make(map[string]struct{})}
}

func (d *OpenDocs) Add(
	uri string,
) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.uris[uri] = struct{}{}
}

func (d *OpenDocs) Remove(
	uri string,
) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.uris, uri)
}

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
