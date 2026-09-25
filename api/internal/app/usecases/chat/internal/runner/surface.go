// Package runner (file surface.go) decides which VIEW a spawn lands on, and
// what that costs the process it is about to fork.
//
// A surface is WHERE the user is looking — Crowbar's own chat, or the
// provider's own CLI (design spec 2.5). The descriptor declares, per surface,
// which delivery CHANNEL carries that surface's facts, and that one
// declaration is the whole of what this file reads: no provider is named
// here, and none may be.
package runner

import (
	"context"
	"fmt"
	"sync"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// storedSurface is the chat's own durable landing VIEW (domain.Chat.Surface).
//
// A create has no chat row to read yet and answers "" — the provider's
// default. That is not a gap: the placed create path (tree.CreateChat) mints
// the chat WITH its surface and then reaches spawnRunner with create=false,
// which is the only way a non-default surface is ever established. Reading it
// here rather than at the create means every later spawn — a restart_tui
// prompt, a model change, a provider switch, a resume after the daemon
// restarted — rebuilds the process on the surface the chat actually lives on.
func (rs *Runners) storedSurface(
	ctx context.Context,
	chatID string,
	create bool,
) (string, error) {
	if create {
		return "", nil
	}
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: spawn runner: chat surface: %w", err)
	}
	return chat.Surface, nil
}

// surfaceForSpawn is the surface this spawn actually lands on: the chat's own
// stored choice when the descriptor declares it launchable, otherwise the
// provider's default ("").
//
// The stored value outlives a provider switch, so this DEGRADES rather than
// refuses: a chat born on one provider's terminal and switched to a provider
// that declares no such landing surface must still come up, on that
// provider's default face, instead of being stuck unspawnable.
func surfaceForSpawn(descriptor engineagents.Agent, stored string) string {
	if stored == "" || !descriptor.SurfaceStartHere(stored) {
		return ""
	}
	return stored
}

// apiTransportDrivesSurface reports whether a chat living on `surface` needs
// this descriptor's api connection opened at all.
//
// One fed by hooks is fed by the CLI's own PTY: an api connection beside it
// would be a second process on the session (one channel per runner).
//
// Absence is not a decision: "" (the provider's default landing) and a
// descriptor with no surfaces: block at all both answer true, so every spawn
// that predates this keeps the connection it always had.
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
) []string {
	if !apiTransportDrivesSurface(descriptor, surfaceForSpawn(descriptor, storedSurface)) {
		return nil
	}
	return rs.applyAPITransport(ctx, runnerID, providerID, descriptor, tctx, resumeContext)
}

// surfaceRegistry is the in-process mirror of domain.Chat.Surface, keyed by
// RUNNER — the CURRENT surface, which is what everything downstream of a
// spawn actually asks about.
//
// The durable field is the truth and this is a copy of it, written in the
// same breath by every path that writes it: seeded at spawn from the chat's
// own stored surface, and moved by SwitchToTerminal/SwitchToNative.
//
// It exists because the read is PER EVENT: surfaceGated (turn/ingest.go) asks
// it for every streamed delta codex declares surfaces: on, and the read-model
// hit that would otherwise cost is the same one the descriptor cache exists
// to avoid — measured at over a millisecond a call, enough to make live
// streaming visibly stall.
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

// get is runnerID's current surface, and answers SurfaceChat for a runner it
// has never been told about. That default is load-bearing: every spawn that
// predates surfaces lands on the provider's own face, which IS Crowbar's
// chat, and reading those as the terminal would gate every chat-only event
// off for all of them.
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
