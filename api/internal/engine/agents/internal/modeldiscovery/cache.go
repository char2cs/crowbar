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

const discoverCacheSubdir = "model-discover-cache"

const (
	ttl                 = 5 * time.Minute
	maxConcurrentProbes = 4
)

type probeFunc func(context.Context, *spec.Descriptor, models.ProbeOptions, exec.Acquire) ([]Model, error)

type manifestProbeFunc func(ctx context.Context, man *spec.ModelManifestSpec, embedded []byte, homeDir string, fetchEnabled bool) ([]Model, error)

type entry struct {
	models         []string
	efforts        map[string][]string
	defaultModel   string
	defaultEfforts map[string]string
	fingerprint    string
	fetchedAt      time.Time
	refreshing     bool
	resolved       bool
}

type Cache struct {
	mu        sync.Mutex
	entries   map[string]entry
	slots     chan struct{}
	probe     probeFunc
	manifest  manifestProbeFunc
	lifecycle context.Context
	stop      context.CancelFunc
	inflight  sync.WaitGroup
	closed    bool
}

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

func (c *Cache) DefaultModel(id string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.entries[id].defaultModel
}

func (c *Cache) DefaultEffort(id, model string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.entries[id].defaultEfforts[model]
}

func (c *Cache) Store(id string, found []Model) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[id] = entry{
		models: idsOf(found), efforts: effortsOf(found), defaultEfforts: defaultEffortsOf(found),
		defaultModel: defaultOf(found), fetchedAt: time.Now(), resolved: true,
	}
}

func (c *Cache) Refresh(d *spec.Descriptor, home string) {
	c.seedDiscoverFromDisk(d, home)
	c.fork(func() { c.attemptRefresh(d, home) })
}

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

func warnFailed(lifecycle context.Context, id string, err error) {
	if err == nil || lifecycle.Err() != nil {
		return
	}
	slog.Warn("modeldiscovery: catalogue refresh failed", "provider", id, "err", err)
}

func manifestFingerprint(url string, fetchEnabled bool) string {
	return fmt.Sprintf("manifest:%s:%v", url, fetchEnabled)
}

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

func discoverCachePath(home, id string) string {
	return filepath.Join(home, discoverCacheSubdir, id+".json")
}

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

func fingerprint(cmd string) string {
	path := exec.Executable(cmd, os.Environ())
	info, err := os.Stat(path)
	if err != nil {
		return path
	}
	return fmt.Sprintf("%s:%d:%d", path, info.Size(), info.ModTime().UnixNano())
}
