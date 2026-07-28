package branchreview

import (
	"slices"
	"sync"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// outlineCacheEntries caps the cache, matching the discipline of the engine's
// summary cache: keys are exact, so a stale entry is impossible by construction
// and no TTL is needed. The cap exists only so a long session moving across many
// workspaces, branches and commits cannot grow the map without bound. Oldest key
// wins eviction; 32 covers the handful of (workspace, base ref) pairs a session
// realistically cycles between.
const outlineCacheEntries = 32

type outlineKey struct {
	repoPath string
	ref      string
	headSHA  string
}

// outlineCache memoises a branch's hunk geometry, which costs a full streamed
// read of the diff to produce — 46 MB of patch on the perf fixture — and is
// fetched afresh every time the review is opened.
//
// The key is the engine summary cache's key exactly: (repoPath, ref, headSHA).
// That key is only EXACT while the answer is a function of two immutable trees,
// which for an outline means a working tree with nothing to add to the ref->HEAD
// diff. Editing a line inside an already-modified file moves the hunk geometry
// without moving HEAD and without changing what git status reports, so no key
// built from those could tell the two apart. Rather than invent a second,
// weaker scheme that would silently serve a stale outline, the caller is
// required to establish that condition before offering a key at all — see
// cacheableOutlineKey — and a working tree that has moved simply recomputes.
//
// This is not the loss it looks like: an outline read is user-initiated, never
// a tick, so a miss costs one streamed diff at the moment the reader asked for
// it. What the cache buys is the common case it is exact for — reviewing a
// finished branch, where the tree is clean and every reopen, tab switch and
// remount would otherwise re-stream the whole diff.
type outlineCache struct {
	mu      sync.Mutex
	entries map[outlineKey][]gitdomain.FileOutline
	order   []outlineKey
}

// get returns the outline for key, calling load exactly once per distinct key
// that is not already cached. load runs outside the mutex: it shells out to git
// and holding a process-wide lock across a subprocess would serialise every
// repo in the daemon behind the slowest one. Two concurrent misses on one key
// therefore both compute, which costs a duplicate diff and never a wrong
// answer. A failed load is not stored, so the next caller retries rather than
// inheriting the failure.
//
// Each caller gets its own copy of the outer slice, so appending to or
// reordering the result cannot corrupt the entry. The per-file Hunks arrays are
// shared with every other holder and must be treated as read-only.
func (c *outlineCache) get(
	key outlineKey,
	load func() ([]gitdomain.FileOutline, error),
) ([]gitdomain.FileOutline, error) {
	if files, ok := c.lookup(key); ok {
		return files, nil
	}
	files, err := load()
	if err != nil {
		return nil, err
	}
	c.store(key, files)
	return slices.Clone(files), nil
}

func (c *outlineCache) lookup(
	key outlineKey,
) ([]gitdomain.FileOutline, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	files, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	return slices.Clone(files), true
}

func (c *outlineCache) store(
	key outlineKey,
	files []gitdomain.FileOutline,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[outlineKey][]gitdomain.FileOutline, outlineCacheEntries)
	}
	if _, exists := c.entries[key]; !exists {
		c.order = append(c.order, key)
	}
	c.entries[key] = files
	for len(c.order) > outlineCacheEntries {
		delete(c.entries, c.order[0])
		c.order = c.order[1:]
	}
}
