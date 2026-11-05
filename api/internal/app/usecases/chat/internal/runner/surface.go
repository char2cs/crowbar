// Package runner (file surface.go) decides which VIEW (Crowbar's chat or the
// provider's CLI) a spawn lands on. It reads only the descriptor's per-surface
// channel declaration; no provider is named here, and none may be.
package runner

import (
	"context"
	"sync"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// surfaceForSpawn is the stored surface when the descriptor can launch it, else the
// default (""). It DEGRADES rather than refuses: the stored value outlives a switch to
// a provider without that surface.
func surfaceForSpawn(descriptor engineagents.Agent, stored string) string {
	if stored == "" || !descriptor.SurfaceStartHere(stored) {
		return ""
	}
	return stored
}

// apiTransportDrivesSurface reports whether a chat on `surface` needs the api
// connection; a hooks-fed surface is fed by the PTY (one channel per runner). "" or
// no surfaces: block answers true.
func apiTransportDrivesSurface(descriptor engineagents.Agent, surface string) bool {
	if surface == "" {
		return true
	}
	return descriptor.SurfaceChannel(surface) != string(engineagents.ChannelHooks)
}

// apiTransportForSurface is spawnRunner's applyAPITransport, gated on the
// chat's surface: nothing is started, and nil is returned, when the surface
// the chat lives on is driven by the hooks channel instead.
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
	return rs.applyAPITransport(ctx, runnerID, providerID, descriptor, tctx, resumeContext)
}

// surfaceRegistry mirrors domain.Chat.Surface per runner, written with it at
// spawn and on a switch: surfaceGated reads it on every streamed delta, where
// the read model's millisecond a call would stall live streaming.
type surfaceRegistry struct {
	mu    sync.Mutex
	byRun map[string]string
}

func newSurfaceRegistry() *surfaceRegistry {
	return &surfaceRegistry{byRun: make(map[string]string)}
}

// set records runnerID's current surface. "" (the provider's own default
// landing) is stored as-is and READ as the chat surface — see get.
func (r *surfaceRegistry) set(runnerID, surface string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byRun[runnerID] = surface
}

// get is runnerID's current surface, SurfaceChat for an unknown runner: the default
// landing is Crowbar's chat, and reading it as terminal would gate off chat-only events.
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
