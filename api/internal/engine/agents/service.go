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
	injected       *registry.Registry
	acceptOverride func(raw []byte) bool

	resolved    sync.Mutex
	descriptors map[string]descriptorCacheEntry

	discovery *modeldiscovery.Cache

	manifestFetchMu sync.RWMutex
	manifestFetch   func() bool
}

type descriptorCacheEntry struct {
	agent           Agent
	overrideModTime time.Time
}

type serviceOpts struct {
	lifecycle      context.Context
	acceptOverride func(raw []byte) bool
}

type Option func(*serviceOpts)

func WithLifecycle(ctx context.Context) Option {
	return func(o *serviceOpts) { o.lifecycle = ctx }
}

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

func (s *service) SetManifestFetchEnabled(enabled func() bool) {
	s.manifestFetchMu.Lock()
	defer s.manifestFetchMu.Unlock()
	s.manifestFetch = enabled
}

func (s *service) manifestFetchAllowed() bool {
	s.manifestFetchMu.RLock()
	defer s.manifestFetchMu.RUnlock()
	if s.manifestFetch == nil {
		return false
	}
	return s.manifestFetch()
}

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
