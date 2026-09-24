package turn

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

func (t *Turns) IngestHook(
	ctx context.Context,
	runnerID string,
	provider string,
	canonicalEvent string,
	rawPayload []byte,
) error {
	// Check the startup barrier BEFORE the repository. Once recordRunner commits,
	// the row is visible, but the barrier deliberately remains installed through
	// ordered replay; consulting only the repository here would let a later hook
	// overtake the buffered session_start/user_prompt batch.
	if handled, err := t.pendingHooks.Enqueue(runnerID, provider, canonicalEvent, rawPayload); handled {
		return err
	}
	return t.ingestHookNow(ctx, runnerID, provider, canonicalEvent, rawPayload)
}

func (t *Turns) ingestUserPromptInterlocked(
	ctx context.Context,
	runnerID, provider, canonicalEvent string,
	rawPayload []byte,
) error {
	for {
		runner, err := t.runnerStore.Get(ctx, runnerID)
		if err != nil {
			if errors.Is(err, agentrunner.ErrNotFound) {
				return nil
			}
			return fmt.Errorf("agent: ingest hook: runner: %w", err)
		}
		if runner.CurrentChatID == "" {
			return nil
		}
		chatID := runner.CurrentChatID
		unlock := t.turnStarts.Lock(chatID)
		current, err := t.runnerStore.Get(ctx, runnerID)
		if err != nil {
			unlock()
			if errors.Is(err, agentrunner.ErrNotFound) {
				return nil
			}
			return fmt.Errorf("agent: ingest hook: refresh runner under turn-start interlock: %w", err)
		}
		if current.CurrentChatID != chatID {
			unlock()
			// Placement changed before the lock. Retry against the durable current
			// chat, or drop on the next iteration if the runner was displaced.
			continue
		}
		err = t.ingestResolvedHook(ctx, current, provider, canonicalEvent, rawPayload)
		unlock()
		return err
	}
}

func (t *Turns) ingestResolvedHook(
	ctx context.Context,
	runner engineagents.Runner,
	provider string,
	canonicalEvent string,
	rawPayload []byte,
) error {
	runnerID := runner.ID

	crowbarHome, _, _, _, err := t.ws.WorktreeDir(ctx, runner.WorkspaceID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: worktree dir: %w", err)
	}

	// The runner is the source of truth for which provider spawned this CLI. The
	// hook's self-reported provider is only a guard against a mis-authored descriptor.
	if provider != "" && provider != runner.ProviderID {
		slog.WarnContext(ctx, "agent: ingest hook: provider mismatch",
			"hook_provider", provider, "runner_provider", runner.ProviderID, "runner_id", runnerID)
	}

	descriptor, err := t.agents.Get(ctx, crowbarHome, runner.ProviderID)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: resolve descriptor: %w", err)
	}

	// Telemetry arrives over the same relay as a hook — a command, JSON on stdin,
	// scoped to the same segment — but it is not a conversation event. It carries
	// no session, describes no turn, and must not be run through the ownership
	// guard, which asks a question about conversations.
	if canonicalEvent == engineagents.HookTelemetry {
		return t.handleTelemetry(ctx, runner, descriptor, rawPayload)
	}

	// One call, deliberately: decode, ownership-check and map are fused in the
	// engine so the middle step cannot be skipped. It is the guard that stops a
	// provider's own internal session being filed as the user's conversation, and
	// a guard a caller can forget to call is one that will eventually be forgotten.
	//
	// channelFor(ctx), not canonicalEvent's declared transport: a channel-scoped
	// event's shape arrives per MESSAGE, not per event declaration (design spec
	// docs/plans/2026-09-22-descriptor-channel-split.md, 1.1) — the same fact
	// ownerDropsThisDelivery below already reads off this ctx, for a different
	// guard.
	//
	// Every failure here DROPS the hook rather than failing it. A hook must never
	// break the vendor CLI's turn.
	ev, err := descriptor.ParseHook(canonicalEvent, rawPayload, channelFor(ctx))
	if err != nil {
		switch {
		case errors.Is(err, engineagents.ErrForeignConversation):
			slog.DebugContext(ctx, "agent: ingest hook: dropping a hook that is not this CLI's own conversation",
				"reason", err, "event", canonicalEvent,
				"provider", runner.ProviderID, "runner_id", runnerID)
		case errors.Is(err, engineagents.ErrHookUndeclared):
			// ERROR, not WARN (P5, design spec F2): this is the exact shape a
			// channel/payload mismatch takes — a delivery landing on a channel
			// its own event never declared a block for — and it used to drop
			// silently enough that a harness bug shipping this same mismatch
			// went unnoticed (docs/plans/2026-09-22-descriptor-channel-
			// split.md). Still non-fatal to the CLI's own turn — "Every
			// failure here DROPS the hook rather than failing it" above is
			// unconditional, a hook relay's exit code must never deny a real
			// tool call over a Crowbar-side mapping gap — but ERROR is what
			// makes it an operator-actionable anomaly instead of routine
			// noise a WARN-level log gets filtered alongside.
			slog.ErrorContext(ctx, "agent: ingest hook: provider does not map this event on this channel",
				"event", canonicalEvent, "channel", channelFor(ctx),
				"provider", runner.ProviderID, "runner_id", runnerID)
		case errors.Is(err, engineagents.ErrRequiredFieldMissing):
			// ERROR (design spec 2.3): a field the descriptor itself declared
			// required resolved to nothing against a REAL delivered payload —
			// exactly the class of latent mismatch this migration exists to
			// surface (the tool||tool_name permission bug shipped invisibly
			// this way). Still non-fatal to the CLI's own turn, same as above.
			slog.ErrorContext(ctx, "agent: ingest hook: required field resolved to nothing",
				"err", err, "event", canonicalEvent, "channel", channelFor(ctx),
				"provider", runner.ProviderID, "runner_id", runnerID)
		default:
			slog.WarnContext(ctx, "agent: ingest hook: parse payload",
				"err", err, "event", canonicalEvent, "runner_id", runnerID)
		}
		return nil
	}

	if t.surfaceGated(runnerID, descriptor, ev.Kind) {
		// design spec P6b tag 2: the descriptor's own surfaces: says this event
		// is not worth ingesting on the view currently in front of the user
		// (Runners.ShowingNativeView) — see surfaceGated's own doc comment. NO
		// CROWBAR-SIDE VETO: whatever the descriptor names is honoured as-is;
		// TestSurfaceGatedEvents_AreReportedLoudly (descriptor package) is the
		// visibility side of that, not a rejection here.
		slog.DebugContext(ctx, "agent: ingest hook: dropping an event gated off the surface in front of the user",
			"event", ev.Kind, "runner_id", runnerID, "provider", runner.ProviderID)
		return nil
	}

	if t.ownerDropsThisDelivery(ctx, runnerID, descriptor, ev.Kind) {
		// A hooks delivery of an event this descriptor declares owner: api
		// (design spec P6b tag 1), for a runner with a live, DISPATCHED api
		// connection right now: pumpAPIConn (apiconn.go) is already forwarding
		// exactly this event kind from that connection's own driver —
		// TransportFor is how it decides which kinds are its to forward. This
		// hooks copy is the mirror problem: every api-transport spawn ALSO
		// forks a real, hooks-wired companion PTY on the SAME session
		// (attach.go calls it "a known gap"), and spawn.Inject applies the
		// descriptor's full hook set to it regardless of TransportFor — that
		// distinction is invisible to the actual CLI process, which just fires
		// whatever it is configured with. Recording this copy too would
		// duplicate whatever the api connection already reported, under a
		// delivery id hookDeliveries' retry journal has never seen (it is a
		// genuinely separate delivery, not a retry of one).
		//
		// inflight.FromAPITransport(ctx) is what tells the two deliveries apart.
		// Without it this guard cannot distinguish "a hooks POST echoing an
		// owner: api event" from "the api-transport delivery of that SAME
		// event, arriving through this exact call" — pumpAPIConn's own
		// IngestHook calls satisfy owner==api and HasLiveAPIConnection==true
		// just as thoroughly as the companion PTY's hooks copy does, so an
		// unmarked call used to drop BOTH: session_start through turn_stop
		// reported successful ingestion while the ledger never gained a single
		// turn — confirmed live, the "Codex still not worky" bug.
		slog.DebugContext(ctx, "agent: ingest hook: dropping a hooks-delivered copy of an owner-declared event",
			"event", ev.Kind, "runner_id", runnerID, "provider", runner.ProviderID)
		return nil
	}

	if t.namesAnotherConversation(ctx, runner, ev) {
		// Not necessarily foreign: a codex-shaped provider pushes a spawned
		// subagent's OWN complete turn/item stream over this SAME connection,
		// carrying the CHILD's own thread id as session_id — see
		// routeNestedSubagentEvent's own doc. Only once that returns false is
		// this genuinely another conversation to drop.
		if routed, err := t.routeNestedSubagentEvent(ctx, runner, ev); routed {
			return err
		}
		slog.DebugContext(ctx,
			"agent: ingest hook: dropping an event that names another conversation",
			"event", ev.Kind, "event_session", ev.SessionID,
			"runner_session", runner.CurrentSession,
			"runner_id", runnerID, "provider", runner.ProviderID)
		return nil
	}

	// Where does this conversation's own transcript stand RIGHT NOW? Asked on every
	// hook and answered once per file: a session Crowbar has not been watching must
	// start at the file's end, or a resumed conversation's whole history would be
	// replayed into the record as though the agent had just said all of it. Asked
	// here rather than on the announcement alone because a daemon that restarts
	// mid-session never sees that session's announcement.

	switch ev.Kind {
	case engineagents.HookSessionStart:
		return t.runners.HandleSessionStart(ctx, runner, ev)
	case engineagents.HookUserPrompt, engineagents.HookTurnStop, engineagents.HookTurnFailed:
		return t.handleTurn(ctx, runner, descriptor, ev)
	case engineagents.HookMessageDelta, engineagents.HookReasoningDelta,
		engineagents.HookIdle, engineagents.HookToolOutputDelta,
		engineagents.HookPlanUpdate,
		engineagents.HookToolPre, engineagents.HookToolPost, engineagents.HookToolFail,
		engineagents.HookSubagentPre, engineagents.HookSubagentPost,
		engineagents.HookNotification, engineagents.HookPermission,
		engineagents.HookElicitation,
		engineagents.HookCompactPre, engineagents.HookCompactPost:
		return t.handleObservation(ctx, runner, descriptor, ev, rawPayload)
	case engineagents.HookSessionEnd:
		// A session ending is already observed authoritatively by the PTY exit
		// reconcile, which runs whether or not the CLI got to fire a hook. Acting
		// on it here as well would close a turn twice.
		return nil
	}
	return nil
}

func (t *Turns) ReplayStartupHook(
	runnerID string,
	hook inflight.Hook,
) {
	replayCtx := context.Background()
	if hook.DeliveryID != "" {
		replayCtx = inflight.WithDeliveryID(replayCtx, hook.DeliveryID)
	}
	if err := t.ingestHookNow(
		replayCtx, runnerID, hook.Provider, hook.CanonicalEvent, hook.RawPayload,
	); err != nil {
		slog.Error("agent: replay startup hook (best-effort, continuing)",
			"runner_id", runnerID, "event", hook.CanonicalEvent, "err", err)
		return
	}
	if hook.DeliveryID == "" {
		return
	}
	if err := t.hookDeliveries.Complete(
		hook.DeliveryDir, hook.DeliveryID, hook.DeliveryHash, time.Now(),
	); err != nil {
		slog.Error("agent: persist replayed startup hook delivery (effects already committed)",
			"runner_id", runnerID, "delivery_id", hook.DeliveryID, "err", err)
	}
}

// channelFor is the delivery channel THIS ingest call actually arrived on —
// pumpAPIConn (apiconn.go) marks its own ctx (inflight.WithAPITransport)
// before calling IngestHook; an HTTP hook relay POST marks nothing, so the
// zero value here is hooks. It is the SAME origin marker ownerDropsThisDelivery
// already reads, for a different guard — see that function's own doc comment
// for why only this ORIGIN, never an event's static Transport (or, since
// design spec P6b, its declared owner:), can tell the two deliveries apart.
func channelFor(ctx context.Context) engineagents.Channel {
	if inflight.FromAPITransport(ctx) {
		return engineagents.ChannelAPI
	}
	return engineagents.ChannelHooks
}

// ownerDropsThisDelivery reports whether THIS CALL is a redundant echo of an
// event the descriptor's own owner: declaration (design spec P6b tag 1) says
// another, live, DISPATCHED channel already reports. Never true for that
// other channel's OWN delivery (inflight.FromAPITransport / channelFor),
// since owner and liveness are both true for that call as well; only the
// ORIGIN of this specific delivery tells the two apart. See the call site's
// own comment for the full mechanism and the bug an unmarked check caused.
//
// owner: replaces ONLY apiOwnsThisEvent's old static
// descriptor.TransportFor(canonical) == "api" guess — the chat-theft root
// cause (design spec 1.1: transport is declared statically per event, but
// shape arrives per message). Declaring an owner says who is AUTHORITATIVE;
// it does NOT say whether that owner actually carried THIS turn, which is
// why the liveness pair below is unconditional, exactly as it was before:
//
// HasDispatchedOverAPI, not just HasLiveAPIConnection: a connection can be
// live yet have carried NOTHING of this runner's own doing — the spawn that
// created it may have handed its opening prompt to the companion PTY's own
// argv instead (submitPromptOverAPI's replacement-spawn fallback, prompts.go).
// A connection nothing has been dispatched to has nothing of its own to echo,
// so treating its mere existence as proof the api side already reports this
// turn was dropping the companion PTY's hooks — the turn's ONLY record — as a
// presumed duplicate of a report that was never actually made. Confirmed
// live: a message answered normally by the CLI never appeared in the ledger
// at all, api transport dutifully "covering" a turn it was never asked to
// carry. This is the zero-writer incident design spec P6b requires stay
// impossible — see TestRegression_ALiveButUndispatchedConnectionNever
// MakesTheCompanionPTYsHooksRedundant (ingest_internal_test.go).
//
// Only owner: api is ever checked against liveness: there is no "hooks
// channel is live" signal distinct from "the runner exists at all" — a
// hooks-transport PTY IS the runner — so an event whose owner: names hooks
// (or declares none) is never dropped by this function.
func (t *Turns) ownerDropsThisDelivery(
	ctx context.Context, runnerID string, descriptor engineagents.Agent, canonical string,
) bool {
	owner := descriptor.EventOwner(canonical)
	if owner == engineagents.OwnerEither {
		return false
	}
	if string(channelFor(ctx)) == owner {
		return false
	}
	if owner != engineagents.OwnerAPI {
		return false
	}
	return t.runners.HasLiveAPIConnection(runnerID) && t.runners.HasDispatchedOverAPI(runnerID)
}

// surfaceGated reports whether canonical opts OUT of the surface currently in
// front of the user — design spec P6b tag 2. The event's own surfaces: list
// is checked BEFORE ever asking ShowingNativeView: most events declare none
// (nil means every surface — "nothing changes unless a descriptor opts in"),
// so the common case never touches the Runners port at all.
//
// NO CROWBAR-SIDE VETO: whatever the descriptor names is honoured as-is, even
// a surface where this event is the only writer of a ledger fact — that is
// reported (TestSurfaceGatedEvents_AreReportedLoudly, descriptor package),
// never rejected here.
//
// ShowingNativeView is only accurate for a NON-HOTSWAP provider (codex): a
// hotswap provider's terminal is a second window onto a session Crowbar
// still drives, so it never calls SwitchToTerminal and this reads false even
// while the TUI is on screen — inert for claude until that signal covers
// hotswap (design spec P6b's own stated limit).
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

func (t *Turns) ingestHookNow(
	ctx context.Context,
	runnerID string,
	provider string,
	canonicalEvent string,
	rawPayload []byte,
) error {
	if canonicalEvent == "user_prompt" {
		return t.ingestUserPromptInterlocked(ctx, runnerID, provider, canonicalEvent, rawPayload)
	}
	runner, err := t.runnerStore.Get(ctx, runnerID)
	if err != nil {
		if errors.Is(err, agentrunner.ErrNotFound) {
			// A hook from a runner we do not know (or one whose PTY has already died):
			// ignore it. Never resurrect a runner from a hook.
			return nil
		}
		return fmt.Errorf("agent: ingest hook: runner: %w", err)
	}
	return t.ingestResolvedHook(ctx, runner, provider, canonicalEvent, rawPayload)
}

func (t *Turns) chatForRunner(
	ctx context.Context,
	runner engineagents.Runner,
) (domain.Chat, bool, error) {
	if runner.CurrentChatID == "" {
		// The runner is placed NOWHERE: Crowbar has taken it off its chat and is killing
		// it (a switch, an eviction, a chat deleted under it), and a SIGTERM'd CLI keeps
		// talking for a moment. Its turns belong to nobody, and nowhere is never looked up
		// — GetChat("") would miss and trigger agentchat's lazy self-heal, replaying the
		// ENTIRE event log, on every hook of a dying CLI.
		return domain.Chat{}, false, nil
	}
	chat, err := t.chats.GetChat(ctx, runner.CurrentChatID)
	if err != nil {
		if errors.Is(err, agentchat.ErrNotFound) {
			// The chat was deleted out from under the CLI (which is still dying). A turn
			// typed into a chat the user has just removed goes nowhere, by design.
			return domain.Chat{}, false, nil
		}
		return domain.Chat{}, false, fmt.Errorf("agent: ingest hook: chat: %w", err)
	}
	return chat, true, nil
}
