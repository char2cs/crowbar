package agents

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/modeldiscovery"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type service struct {
	injected *registry.Registry
	// acceptOverride is WithOverrideCheck's predicate, nil to admit all.
	acceptOverride func(raw []byte) bool

	resolved sync.Mutex
	// descriptors caches Get per (homeDir, id): the hook recorder calls it per
	// streamed delta, and a full Resolve costs a millisecond. A Stat of the
	// override path keeps an edited override picked up without a restart.
	descriptors map[string]descriptorCacheEntry

	// discovery is the shared, provider-keyed cache behind a model.discover
	// OR model.manifest descriptor's Models()/Efforts() — one instance for
	// every agent this service ever builds, so a refresh triggered by one
	// List/Get is visible to the next, however many *agent values point at
	// it.
	discovery *modeldiscovery.Cache

	// manifestFetchMu guards manifestFetch, the live getter
	// SetManifestFetchEnabled installs — read fresh on every manifest
	// refresh, never latched at construction, so a settings toggle takes
	// effect on the very next refresh rather than needing a restart.
	manifestFetchMu sync.RWMutex
	manifestFetch   func() bool
}

// descriptorCacheEntry is one Get result, held until the on-disk override
// path it was resolved against changes.
type descriptorCacheEntry struct {
	agent Agent
	// overrideModTime is protocol.OverridePath's mtime as of this resolve, or
	// the zero Time when no override file existed then (including every
	// homeDir-less caller). Compared against a fresh Stat on every Get so an
	// override created, edited, or removed on disk invalidates the entry —
	// the zero Time changing to a real one, or a real one changing, are both
	// "this changed" under time.Time.Equal.
	overrideModTime time.Time
}

// serviceOpts is New's option set.
type serviceOpts struct {
	lifecycle      context.Context
	acceptOverride func(raw []byte) bool
}

// Option configures New.
type Option func(*serviceOpts)

// WithLifecycle ties every background model-discovery refresh to ctx: cancelling it stops
// in-flight probes and refuses to store one that raced past it. Omitted, refreshes are bound to
// context.Background().
func WithLifecycle(ctx context.Context) Option {
	return func(o *serviceOpts) { o.lifecycle = ctx }
}

// WithOverrideCheck admits an on-disk descriptor override only when accept
// passes it; a refused override runs the shipped default instead. Omitted,
// every override is loaded as written.
func WithOverrideCheck(accept func(raw []byte) bool) Option {
	return func(o *serviceOpts) { o.acceptOverride = accept }
}

func New(opts ...Option) Agents {
	cfg := serviceOpts{lifecycle: context.Background()}
	for _, o := range opts {
		o(&cfg)
	}
	return &service{
		acceptOverride: cfg.acceptOverride,
		injected:       registry.New(),
		descriptors:    map[string]descriptorCacheEntry{},
		discovery:      modeldiscovery.NewCache(cfg.lifecycle),
	}
}

func (s *service) List(ctx context.Context, homeDir string) ([]Agent, error) {
	descriptors, err := protocol.All(ctx, homeDir, s.acceptOverride)
	if err != nil {
		return nil, err
	}
	out := make([]Agent, 0, len(descriptors))
	for _, d := range descriptors {
		s.refreshModelsIfDeclared(d, homeDir) //nolint:contextcheck // the refresh outlives this request: it runs on the service lifecycle context
		out = append(out, &agent{spec: d, discovery: s.discovery})
	}
	return out, nil
}

func (s *service) Get(ctx context.Context, homeDir, id string) (Agent, error) {
	key := homeDir + "\x00" + id
	modTime := overrideModTime(protocol.OverridePath(homeDir, id))

	s.resolved.Lock()
	cached, ok := s.descriptors[key]
	s.resolved.Unlock()
	if ok && cached.overrideModTime.Equal(modTime) {
		return cached.agent, nil
	}

	d, err := protocol.Resolve(ctx, homeDir, id, s.acceptOverride)
	if err != nil {
		return nil, err
	}
	s.refreshModelsIfDeclared(d, homeDir) //nolint:contextcheck // the refresh outlives this request: it runs on the service lifecycle context
	a := &agent{spec: d, discovery: s.discovery}

	s.resolved.Lock()
	s.descriptors[key] = descriptorCacheEntry{agent: a, overrideModTime: modTime}
	s.resolved.Unlock()
	return a, nil
}

// refreshModelsIfDeclared kicks off a background catalogue refresh for a
// descriptor that declares model.discover or model.manifest — a no-op (and
// zero extra cost on every other Get/List call) for one that declares
// neither.
func (s *service) refreshModelsIfDeclared(d *spec.Descriptor, homeDir string) {
	if d.Model == nil {
		return
	}
	if d.Model.Discover != nil {
		s.discovery.Refresh(d, homeDir)
	}
	if d.Model.Manifest != nil {
		s.discovery.RefreshManifest(d, homeDir, protocol.EmbeddedModelManifest(), s.manifestFetchAllowed())
	}
}

// SetManifestFetchEnabled installs the live getter refreshModelsIfDeclared's
// model.manifest half reads before hitting the network.
func (s *service) SetManifestFetchEnabled(enabled func() bool) {
	s.manifestFetchMu.Lock()
	defer s.manifestFetchMu.Unlock()
	s.manifestFetch = enabled
}

// manifestFetchAllowed is false until SetManifestFetchEnabled wires a getter, so resolving a
// descriptor never makes a network call as a side effect.
func (s *service) manifestFetchAllowed() bool {
	s.manifestFetchMu.RLock()
	defer s.manifestFetchMu.RUnlock()
	if s.manifestFetch == nil {
		return false
	}
	return s.manifestFetch()
}

// overrideModTime is the zero Time for an empty path (no override possible)
// or one Stat could not reach (none exists, or a permission error — either
// way, Resolve itself will fail the same Stat too, so there is nothing this
// cache could get more wrong than Resolve already would).
func overrideModTime(path string) time.Time {
	if path == "" {
		return time.Time{}
	}
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// Close joins this service's model-discovery cache — see Agents.Close.
func (s *service) Close() {
	s.discovery.Close()
}

func (s *service) RecordInjection(runnerID string, docs ...string) {
	s.injected.SetInjected(runnerID, docs...)
}

func (s *service) ConsumeInjectedPrefix(runnerID, text string) (string, bool) {
	return s.injected.ConsumePrefix(runnerID, text)
}

func (s *service) ForgetRunner(runnerID string) {
	s.injected.Forget(runnerID)
}
