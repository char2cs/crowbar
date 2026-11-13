package modeldiscovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// discoverCacheSubdir holds model.discover:'s on-disk seed, one JSON file per
// descriptor id. It is read synchronously on a cold cache because a live probe has
// no embedded bundle to fall back on, so a fresh daemon would otherwise list nothing.
const discoverCacheSubdir = "model-discover-cache"

// ttl bounds how long a resolved catalogue is trusted; maxConcurrentProbes is this
// package's own fork budget, separate from the daemon's per-chat probe budget because
// discovery is provider-level and rarely refreshed.
const (
	ttl                 = 5 * time.Minute
	maxConcurrentProbes = 4
)

// probeFunc is Probe's signature, held as a field so a test can swap in a fake.
type probeFunc func(context.Context, *spec.Descriptor, models.ProbeOptions, exec.Acquire) ([]Model, error)

// manifestProbeFunc is ProbeManifest's signature, a field so a test avoids a real URL.
type manifestProbeFunc func(ctx context.Context, man *spec.ModelManifestSpec, embedded []byte, homeDir string, fetchEnabled bool) ([]Model, error)

type entry struct {
	models       []string
	efforts      map[string][]string
	defaultModel string
	// defaultEfforts holds only the models whose source stated a default effort.
	defaultEfforts map[string]string
	fingerprint    string
	fetchedAt      time.Time
	refreshing     bool
	// resolved is true once store() has run for this id. len(models) cannot tell
	// "never resolved" from "resolved to zero rows", and re-seeding on the latter
	// would clobber a landing async refresh.
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
	// lifecycle bounds every background probe/fetch. It is deliberately not the
	// triggering request's ctx, which must not cut a refresh short when it returns.
	lifecycle context.Context
	// stop cancels lifecycle and inflight joins every forked goroutine; only the
	// join guarantees no disk write lands under home after Close returns.
	stop     context.CancelFunc
	inflight sync.WaitGroup
	closed   bool
}

// NewCache constructs a Cache whose background work is bounded by lifecycle:
// cancelling it stops every probe/fetch and discards results that raced past it.
// Pass a long-lived owner context, not context.Background().
func NewCache(lifecycle context.Context) *Cache {
	ctx, cancel := context.WithCancel(lifecycle)
	return &Cache{
		entries:   map[string]entry{},
		slots:     make(chan struct{}, maxConcurrentProbes),
		probe:     guardedProbe,
		manifest:  ProbeManifest,
		lifecycle: ctx,
		stop:      cancel,
	}
}

// Close cancels every background refresh and BLOCKS until each has returned, disk
// write included: a refresh past its fetch makes no more ctx checks, so only the join
// keeps writes out of a released home. Idempotent; fork refuses new work once closed.
func (c *Cache) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.mu.Unlock()

	c.stop()
	c.inflight.Wait()
}

// fork runs one background refresh under the Close join, or not at all once closed.
// The closed check and Add share mu with Close, so no Add can follow Wait.
func (c *Cache) fork(run func()) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.inflight.Add(1)
	c.mu.Unlock()

	go func() {
		defer c.inflight.Done()
		run()
	}()
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

// DefaultModel is the id the source flagged as default (default_when), or "" when
// it flagged none or nothing resolved yet. "" never means "use the first model".
func (c *Cache) DefaultModel(id string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.entries[id].defaultModel
}

// DefaultEffort is the effort the source states model runs at by default, "" when it states none.
func (c *Cache) DefaultEffort(id, model string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.entries[id].defaultEfforts[model]
}

// Store seeds id's cached catalogue directly, bypassing a probe.
func (c *Cache) Store(id string, found []Model) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[id] = entry{
		models: idsOf(found), efforts: effortsOf(found), defaultEfforts: defaultEffortsOf(found),
		defaultModel: defaultOf(found), fetchedAt: time.Now(), resolved: true,
	}
}

// Refresh seeds an unresolved id from disk synchronously, then probes in the
// background when due; readers never block on a fork. A failed probe keeps the old
// catalogue but records the attempt, so a missing CLI is retried once per ttl.
func (c *Cache) Refresh(d *spec.Descriptor, home string) {
	c.seedDiscoverFromDisk(d, home)
	c.fork(func() { c.attemptRefresh(d, home) })
}

// seedDiscoverFromDisk seeds id from the last probe's on-disk catalogue on the
// calling goroutine, so a cold daemon serves last-known models at once. A no-op once
// anything resolved (never clobbers a refresh), without home, or with no file yet.
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
	// ctx.Err() is checked even on a nil err: a cancelled lifecycle never writes,
	// even if the probe raced past cancellation and returned a result.
	if err != nil || ctx.Err() != nil {
		warnFailed(c.lifecycle, d.ID, err)
		c.markAttempted(d.ID, fp)
		return
	}
	c.store(d.ID, found, fp)
	if err := writeDiscoverCache(home, d.ID, found); err != nil {
		slog.Warn("modeldiscovery: discover cache not saved", "provider", d.ID, "err", err)
	}
}

// RefreshManifest is Refresh for model.manifest: it seeds an unresolved id from the
// embedded bundle synchronously, then refreshes from disk cache and network in the
// background. fetchEnabled gates only the network, so disabling it never empties the list.
func (c *Cache) RefreshManifest(d *spec.Descriptor, home string, embedded []byte, fetchEnabled bool) {
	c.seedManifestFromEmbedded(d, embedded)
	c.fork(func() { c.attemptManifestRefresh(d, home, embedded, fetchEnabled) })
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
		warnFailed(c.lifecycle, d.ID, err)
		c.markAttempted(d.ID, fp)
		return
	}
	c.store(d.ID, found, fp)
}

// warnFailed makes a failed refresh visible, since the catalogue otherwise just stays
// empty until the next ttl. A shutdown cancel is not a failure.
func warnFailed(lifecycle context.Context, id string, err error) {
	if err == nil || lifecycle.Err() != nil {
		return
	}
	slog.Warn("modeldiscovery: catalogue refresh failed", "provider", id, "err", err)
}

func manifestFingerprint(url string, fetchEnabled bool) string {
	return fmt.Sprintf("manifest:%s:%v", url, fetchEnabled)
}

// beginRefresh reports whether id is due, marking it in-flight under the same lock
// so a concurrent call skips instead of forking a duplicate. freshFor is the source's
// own TTL (discover's fixed ttl or the manifest's ttl_ms).
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
		defaultEfforts: defaultEffortsOf(found), fingerprint: fp, fetchedAt: time.Now(), resolved: true,
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

func defaultEffortsOf(found []Model) map[string]string {
	out := make(map[string]string, len(found))
	for _, m := range found {
		if m.DefaultEffort != "" {
			out[m.ID] = m.DefaultEffort
		}
	}
	return out
}

func idsOf(found []Model) []string {
	out := make([]string, len(found))
	for i, m := range found {
		out[i] = m.ID
	}
	return out
}

// effortsOf keys each row's effort levels by model id, plus "" (never a real id)
// mapped to the union of all levels, which is what a picker reads before a model
// is chosen.
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

// discoverCachePath is keyed by descriptor id, unlike manifestCachePath's URL hash,
// since a discover source has no URL to key by.
func discoverCachePath(home, id string) string {
	return filepath.Join(home, discoverCacheSubdir, id+".json")
}

// writeDiscoverCache persists a successful probe's catalogue so the next cold
// start has something to seed from. A failure only costs that seed, never this
// run's cached result, so the caller logs it.
func writeDiscoverCache(home, id string, found []Model) error {
	if home == "" {
		return nil
	}
	raw, err := json.Marshal(found)
	if err != nil {
		return fmt.Errorf("modeldiscovery: encode discover cache: %w", err)
	}
	path := discoverCachePath(home, id)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("modeldiscovery: discover cache dir: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("modeldiscovery: write discover cache: %w", err)
	}
	return nil
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
