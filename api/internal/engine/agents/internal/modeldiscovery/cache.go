package modeldiscovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// discoverCacheSubdir holds model.discover:'s own on-disk seed, one JSON file
// per descriptor id — the same "survive a restart" role model-manifest-cache
// already plays for model.manifest:, but read SYNCHRONOUSLY on a cold cache
// (seedDiscoverFromDisk) rather than only in the background: a live probe has
// no embedded bundle to fall back on meanwhile, so without this a fresh
// daemon serves an empty catalogue until the first probe lands.
const discoverCacheSubdir = "model-discover-cache"

// ttl bounds how long a resolved catalogue is trusted before a fresh probe is
// worth its fork; maxConcurrentProbes bounds how many of those forks may run
// at once — this package's own budget, since model discovery is
// provider-level (one entry per descriptor, refreshed rarely) rather than
// the per-chat, never-cached probes the daemon's chat-scoped budget bounds.
const (
	ttl                 = 5 * time.Minute
	maxConcurrentProbes = 4
)

// probeFunc is Probe's own signature, held as a field so a test can swap in a
// fake without a real provider binary to discover against.
type probeFunc func(context.Context, *spec.Descriptor, models.ProbeOptions, exec.Acquire) ([]Model, error)

// manifestProbeFunc is ProbeManifest's own signature, held as a field for the
// same reason probeFunc is — a test swaps in a fake rather than hitting a
// real URL.
type manifestProbeFunc func(ctx context.Context, man *spec.ModelManifestSpec, embedded []byte, homeDir string, fetchEnabled bool) ([]Model, error)

type entry struct {
	models       []string
	efforts      map[string][]string
	defaultModel string
	fingerprint  string
	fetchedAt    time.Time
	refreshing   bool
	// resolved is true once store() has run for this id at least once — the
	// seed check a manifest source's cold-start needs, since len(models)>0
	// alone can't tell "never resolved" from "resolved to legitimately zero
	// rows" (e.g. every row keep_when-filtered out), which would otherwise
	// re-seed and clobber a landing async refresh on every single call.
	resolved bool
}

// Cache holds the last resolved catalogue per provider id. Read methods never
// block on I/O; Refresh forks a probe in the background when the cached
// entry is missing, stale, or the CLI binary itself changed.
type Cache struct {
	mu       sync.Mutex
	entries  map[string]entry
	slots    chan struct{}
	probe    probeFunc
	manifest manifestProbeFunc
	// afterRefresh, set only by a test, runs at the end of attemptRefresh
	// (success or failure) once every side effect — store, disk write — has
	// landed. It exists so a test can block on the ACTUAL completion of a
	// forked refresh (e.g. before a t.TempDir() cleanup would otherwise race
	// its still-running disk write) rather than guessing with a sleep.
	afterRefresh func(id string)
	// lifecycle bounds every background probe/fetch this Cache ever forks —
	// deliberately NOT whatever ctx the List/Get call that triggered a given
	// Refresh happened to carry, which is request-scoped and must not cut a
	// refresh short just because the request that triggered it already
	// returned. attemptRefresh/attemptManifestRefresh derive their own
	// per-call timeout from this one, so cancelling it (a daemon's own
	// shutdown, or a test's teardown) is what actually stops them — never a
	// detached context.Background() that outlives whoever asked for it.
	lifecycle context.Context
}

// NewCache constructs a Cache whose background work is bounded by lifecycle:
// cancelling it stops every in-flight and future probe/fetch this Cache
// forks, and refuses to store or write a result that raced past the
// cancellation (see attemptRefresh). Pass the caller's own long-lived
// context — never context.Background() dressed up as one — or nothing here
// can ever be told to stop.
func NewCache(lifecycle context.Context) *Cache {
	return &Cache{
		entries:   map[string]entry{},
		slots:     make(chan struct{}, maxConcurrentProbes),
		probe:     guardedProbe,
		manifest:  ProbeManifest,
		lifecycle: lifecycle,
	}
}

// Models is a synchronous, I/O-free read: whatever Refresh last resolved, or
// empty when nothing has resolved yet — never an error, since an unresolved
// catalogue is "not yet known", not "known to be empty".
func (c *Cache) Models(id string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.entries[id].models...)
}

func (c *Cache) Efforts(id, model string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.entries[id].efforts[model]...)
}

// DefaultModel is the id the source itself flagged as its default
// (model.discover.default_when) — empty when the source states nothing at
// all, or nothing has resolved yet. Empty NEVER means "fall back to the
// first model in Models()".
func (c *Cache) DefaultModel(id string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.entries[id].defaultModel
}

// Store seeds id's cached catalogue directly, bypassing a probe — used by
// this package's own wiring tests, and available to any future caller that
// already has a resolved list from elsewhere.
func (c *Cache) Store(id string, found []Model) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[id] = entry{
		models: idsOf(found), efforts: effortsOf(found),
		defaultModel: defaultOf(found), fetchedAt: time.Now(), resolved: true,
	}
}

// Refresh seeds id's cache SYNCHRONOUSLY from its on-disk file when nothing
// has resolved yet (seedDiscoverFromDisk — cheap, local, bounded), then probes
// the live catalogue in the background when due, returning immediately either
// way — Models/Efforts callers (Agent.Models, three selection-validation
// gates) must never block on a fork. A probe failure keeps whatever was
// cached before it and still records the attempt, so a CLI that is simply
// not installed is retried at most once per ttl, not once per call.
func (c *Cache) Refresh(d *spec.Descriptor, home string) {
	c.seedDiscoverFromDisk(d, home)
	go c.attemptRefresh(d, home)
}

// seedDiscoverFromDisk is model.discover:'s counterpart of
// seedManifestFromEmbedded: it seeds id's cache from the last catalogue a
// successful probe wrote to disk, on the CALLING goroutine, so a cold daemon
// serves last-known models immediately rather than waiting out the first
// live probe. A no-op once something has already resolved (never clobbers a
// landed or in-flight refresh), for a homeDir-less caller, or when no cache
// file exists yet — the first-ever run on a machine has nothing to seed from,
// which is what the boot warm-up is for.
func (c *Cache) seedDiscoverFromDisk(d *spec.Descriptor, home string) {
	if d == nil || d.Model == nil || d.Model.Discover == nil || home == "" {
		return
	}
	c.mu.Lock()
	seeded := c.entries[d.ID].resolved
	c.mu.Unlock()
	if seeded {
		return
	}
	raw, err := os.ReadFile(discoverCachePath(home, d.ID))
	if err != nil {
		return
	}
	var found []Model
	if err := json.Unmarshal(raw, &found); err != nil {
		return
	}
	c.store(d.ID, found, "")
}

func (c *Cache) attemptRefresh(d *spec.Descriptor, home string) {
	if d == nil || d.Model == nil || d.Model.Discover == nil {
		return
	}
	fp := fingerprint(d.Spawn.Cmd)
	if !c.beginRefresh(d.ID, fp, ttl) {
		return
	}
	if c.afterRefresh != nil {
		defer c.afterRefresh(d.ID)
	}

	ctx, cancel := context.WithTimeout(
		c.lifecycle, time.Duration(d.Model.Discover.EffectiveTimeoutMS())*time.Millisecond,
	)
	defer cancel()

	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	case <-ctx.Done():
		c.markAttempted(d.ID, fp)
		return
	}

	found, err := c.probe(ctx, d, models.ProbeOptions{Cwd: home, Env: os.Environ()}, nil)
	// ctx.Err() is checked even on a nil err: a probe that raced past its own
	// cancellation and returned a result anyway (or simply never looked) must
	// still not land — a cancelled lifecycle NEVER writes, no matter what the
	// probe itself did.
	if err != nil || ctx.Err() != nil {
		c.markAttempted(d.ID, fp)
		return
	}
	c.store(d.ID, found, fp)
	writeDiscoverCache(home, d.ID, found)
}

// RefreshManifest resolves id's model.manifest: catalogue — the manifest
// counterpart of Refresh, over a fetch instead of a live probe. It first
// seeds the cache SYNCHRONOUSLY from the embedded bundle alone when nothing
// has resolved yet (cheap, local, no I/O), so Models()/Efforts() never read
// empty just because the background half (disk cache + network) has not
// finished — then kicks off that background half exactly like Refresh does.
// fetchEnabled gates the network half only; disk cache and embedded always
// apply, so turning it off can only ever fall further back, never empty the
// catalogue.
func (c *Cache) RefreshManifest(d *spec.Descriptor, home string, embedded []byte, fetchEnabled bool) {
	c.seedManifestFromEmbedded(d, embedded)
	go c.attemptManifestRefresh(d, home, embedded, fetchEnabled)
}

func (c *Cache) seedManifestFromEmbedded(d *spec.Descriptor, embedded []byte) {
	if d == nil || d.Model == nil || d.Model.Manifest == nil {
		return
	}
	c.mu.Lock()
	seeded := c.entries[d.ID].resolved
	c.mu.Unlock()
	if seeded {
		return
	}
	doc, ok := parseManifestDoc(embedded)
	if !ok {
		return
	}
	c.store(d.ID, mapManifestDoc(doc, d.Model.Manifest), "")
}

func (c *Cache) attemptManifestRefresh(d *spec.Descriptor, home string, embedded []byte, fetchEnabled bool) {
	if d == nil || d.Model == nil || d.Model.Manifest == nil {
		return
	}
	man := d.Model.Manifest
	fp := manifestFingerprint(man.URL, fetchEnabled)
	freshFor := time.Duration(man.EffectiveTTLMS()) * time.Millisecond
	if !c.beginRefresh(d.ID, fp, freshFor) {
		return
	}

	ctx, cancel := context.WithTimeout(
		c.lifecycle, time.Duration(man.EffectiveTimeoutMS())*time.Millisecond,
	)
	defer cancel()

	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	case <-ctx.Done():
		c.markAttempted(d.ID, fp)
		return
	}

	found, err := c.manifest(ctx, man, embedded, home, fetchEnabled)
	if err != nil || ctx.Err() != nil {
		c.markAttempted(d.ID, fp)
		return
	}
	c.store(d.ID, found, fp)
}

func manifestFingerprint(url string, fetchEnabled bool) string {
	return fmt.Sprintf("manifest:%s:%v", url, fetchEnabled)
}

// beginRefresh reports whether id is actually due for a refresh, marking it
// in-flight under the same lock so a second concurrent call sees the flag
// and skips rather than racing a duplicate fork. freshFor is the caller's own
// TTL — model.discover's fixed ttl, or model.manifest's own configured
// ttl_ms — so the two sources can be refreshed on different schedules while
// sharing one due-check.
func (c *Cache) beginRefresh(id, fp string, freshFor time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	cur := c.entries[id]
	if cur.refreshing || (cur.fingerprint == fp && time.Since(cur.fetchedAt) < freshFor) {
		return false
	}
	cur.refreshing = true
	c.entries[id] = cur
	return true
}

func (c *Cache) markAttempted(id, fp string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cur := c.entries[id]
	cur.fingerprint = fp
	cur.fetchedAt = time.Now()
	cur.refreshing = false
	c.entries[id] = cur
}

func (c *Cache) store(id string, found []Model, fp string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[id] = entry{
		models: idsOf(found), efforts: effortsOf(found), defaultModel: defaultOf(found),
		fingerprint: fp, fetchedAt: time.Now(), resolved: true,
	}
}

// defaultOf is the id of the first row the source itself flagged as
// default, or "" when none was — never a positional fallback.
func defaultOf(found []Model) string {
	for _, m := range found {
		if m.Default {
			return m.ID
		}
	}
	return ""
}

func idsOf(found []Model) []string {
	out := make([]string, len(found))
	for i, m := range found {
		out[i] = m.ID
	}
	return out
}

// effortsOf keys each row's own effort levels by its model id, PLUS "" —
// never a real model id (renderModel drops any row with an empty one) — set
// to the union of every row's levels. "" is what a caller with no model
// chosen yet reads (provider.resolveEfforts's own "" entry): the union is
// the most defensible answer when the source states no default model to
// read a list from directly, so a picker shown before a model is confirmed
// still offers something sane.
func effortsOf(found []Model) map[string][]string {
	out := make(map[string][]string, len(found)+1)
	seen := make(map[string]bool, 8)
	var union []string
	for _, m := range found {
		out[m.ID] = m.Efforts
		for _, e := range m.Efforts {
			if !seen[e] {
				seen[e] = true
				union = append(union, e)
			}
		}
	}
	out[""] = union
	return out
}

// discoverCachePath is keyed by the descriptor id — this package's own cache
// key everywhere else (Cache.entries), unlike manifestCachePath's URL hash,
// since model.discover: has no second URL a provider id could collide with.
func discoverCachePath(home, id string) string {
	return filepath.Join(home, discoverCacheSubdir, id+".json")
}

// writeDiscoverCache persists a successful probe's catalogue to disk so the
// next cold start has something to seed from. Best-effort: a write failure
// (missing home, read-only fs) only costs the NEXT cold start its synchronous
// seed, never this one's already-cached result.
func writeDiscoverCache(home, id string, found []Model) {
	if home == "" {
		return
	}
	raw, err := json.Marshal(found)
	if err != nil {
		return
	}
	path := discoverCachePath(home, id)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return
	}
	_ = os.WriteFile(path, raw, 0o644) //nolint:gosec // cache file, not a secret
}

// fingerprint identifies the CLI binary a discover: command would run —
// path+size+mtime — so a CLI upgrade invalidates a cached catalogue
// immediately rather than waiting out the ttl.
func fingerprint(cmd string) string {
	path := exec.Executable(cmd, os.Environ())
	info, err := os.Stat(path)
	if err != nil {
		return path
	}
	return fmt.Sprintf("%s:%d:%d", path, info.Size(), info.ModTime().UnixNano())
}
