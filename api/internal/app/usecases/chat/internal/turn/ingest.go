package turn

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// errChatGone is a hook from a runner placed on no chat, or on one deleted
// under it: nothing records it, and a CLI's turn is never resurrected by one.
var errChatGone = fmt.Errorf("agent: hook: the runner's chat is gone: %w", apperr.ErrNotFound)

// Descriptor resolves the descriptor runner's hooks are parsed with.
func (t *Turns) Descriptor(ctx context.Context, runner engineagents.Runner) (engineagents.Agent, error) {
	crowbarHome, _, _, _, err := t.ws.WorktreeDir(ctx, runner.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("agent: hook: worktree dir: %w", err)
	}
	agent, err := t.agents.Get(ctx, crowbarHome, runner.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("agent: hook: resolve descriptor: %w", err)
	}
	return agent, nil
}

// Dispatch records ev, one hook of runner's parsed by agent, on runner's chat.
// The caller holds the chat's lock. It returns once the rows are committed,
// with the stdout the hook's sender must print and the ask its answer is held on.
func (t *Turns) Dispatch(
	ctx context.Context,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
	raw []byte,
	channel engineagents.Channel,
) (reply []byte, ask string, err error) {
	if t.surfaceGated(runner.ID, agent, ev.Kind) {
		return nil, "", nil
	}
	ctx, bound, err := t.withHookSession(ctx, runner, channel)
	if err != nil {
		return nil, "", err
	}
	if namesAnotherConversation(bound, ev) {
		// A child conversation the chat's agent opened rides the same wire; only
		// once it is not one of the chat's open subagents is ev foreign.
		routed, ask, err := t.routeNestedSubagentEvent(ctx, runner, agent, ev, raw)
		if !routed {
			return nil, "", fmt.Errorf("agent: hook: session %s: %w", ev.SessionID, engineagents.ErrForeignConversation)
		}
		return nil, ask, err
	}
	if routeSubagentTool(ev) {
		return nil, "", nil
	}
	switch ev.Kind {
	case engineagents.HookSessionStart:
		return nil, "", t.runners.HandleSessionStart(ctx, runner, ev)
	case engineagents.HookUserPrompt, engineagents.HookTurnStop, engineagents.HookTurnFailed:
		reply, err := t.handleTurn(ctx, runner, agent, ev)
		return reply, "", err
	case engineagents.HookMessageDelta, engineagents.HookReasoningDelta,
		engineagents.HookIdle, engineagents.HookToolOutputDelta,
		engineagents.HookPlanUpdate, engineagents.HookDiffUpdate,
		engineagents.HookToolPre, engineagents.HookToolPost, engineagents.HookToolFail,
		engineagents.HookSubagentPre, engineagents.HookSubagentPost,
		engineagents.HookNotification, engineagents.HookPermission,
		engineagents.HookElicitation,
		engineagents.HookCompactPre, engineagents.HookCompactPost:
		ask, err := t.handleObservation(ctx, runner, agent, ev, raw)
		return nil, ask, err
	}
	// session_end and anything else declared but unhandled: a runner's end is
	// observed by its process exit, which runs whether or not a hook fired.
	return nil, "", nil
}

// surfaceGated reports whether canonical opts OUT of the surface currently in
// front of the user. The descriptor's own surfaces: list is honoured as-is;
// an event that declares none is on every surface, so the common case never
// asks which surface is showing.
func (t *Turns) surfaceGated(runnerID string, descriptor engineagents.Agent, canonical string) bool {
	surfaces := descriptor.EventSurfaces(canonical)
	if len(surfaces) == 0 {
		return false
	}
	surface := engineagents.SurfaceChat
	if t.runners.ShowingNativeView(runnerID) {
		surface = engineagents.SurfaceTerminal
	}
	return !slices.Contains(surfaces, surface)
}

// chatForRunner is the chat runner is on, errChatGone when it is on none or
// that chat was deleted under it (the CLI is still dying).
func (t *Turns) chatForRunner(ctx context.Context, runner engineagents.Runner) (domain.Chat, error) {
	if runner.CurrentChatID == "" {
		return domain.Chat{}, errChatGone
	}
	chat, err := t.chats.GetChat(ctx, runner.CurrentChatID)
	if errors.Is(err, agentchat.ErrNotFound) {
		return domain.Chat{}, errChatGone
	}
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agent: hook: chat: %w", err)
	}
	return chat, nil
}
