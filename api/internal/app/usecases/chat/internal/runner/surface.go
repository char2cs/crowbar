package runner

import (
	"context"
	"sync"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func surfaceForSpawn(descriptor engineagents.Agent, stored string) string {
	if stored == "" || !descriptor.SurfaceStartHere(stored) {
		return ""
	}
	return stored
}

func apiTransportDrivesSurface(descriptor engineagents.Agent, surface string) bool {
	if surface == "" {
		return true
	}
	return descriptor.SurfaceChannel(surface) != string(engineagents.ChannelHooks)
}

func (rs *Runners) apiTransportForSurface(
	ctx context.Context,
	runnerID, providerID string,
	descriptor engineagents.Agent,
	tctx engineagents.TemplateCtx,
	resumeContext string,
	storedSurface string,
) ([]string, error) {
	if !apiTransportDrivesSurface(descriptor, surfaceForSpawn(descriptor, storedSurface)) {
		return nil, nil
	}
	return rs.applyAPITransport(
		ctx, runnerID, providerID, descriptor, tctx, resumeContext, apiIsOnlyChannel(descriptor, storedSurface),
	)
}

func apiIsOnlyChannel(descriptor engineagents.Agent, stored string) bool {
	surface := surfaceForSpawn(descriptor, stored)
	if surface == "" {
		surface = engineagents.SurfaceChat
	}
	return descriptor.SurfaceChannel(surface) == string(engineagents.ChannelAPI)
}

type surfaceRegistry struct {
	mu    sync.Mutex
	byRun map[string]string
}

func newSurfaceRegistry() *surfaceRegistry {
	return &surfaceRegistry{byRun: make(map[string]string)}
}

func (r *surfaceRegistry) set(runnerID, surface string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byRun[runnerID] = surface
}

func (r *surfaceRegistry) get(runnerID string) string {
	if r == nil {
		return engineagents.SurfaceChat
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if s := r.byRun[runnerID]; s != "" {
		return s
	}
	return engineagents.SurfaceChat
}

func (r *surfaceRegistry) drop(runnerID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byRun, runnerID)
}
