package turn

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	// Every failure here DROPS the hook rather than failing it. A hook must never
	// break the vendor CLI's turn.
	ev, err := descriptor.ParseHook(canonicalEvent, rawPayload)
	if err != nil {
		switch {
		case errors.Is(err, engineagents.ErrForeignConversation):
			slog.DebugContext(ctx, "agent: ingest hook: dropping a hook that is not this CLI's own conversation",
				"reason", err, "event", canonicalEvent,
				"provider", runner.ProviderID, "runner_id", runnerID)
		case errors.Is(err, engineagents.ErrHookUndeclared):
			slog.DebugContext(ctx, "agent: ingest hook: provider does not map this event",
				"event", canonicalEvent, "provider", runner.ProviderID, "runner_id", runnerID)
		default:
			slog.WarnContext(ctx, "agent: ingest hook: parse payload",
				"err", err, "event", canonicalEvent, "runner_id", runnerID)
		}
		return nil
	}

	if t.apiOwnsThisEvent(ctx, runnerID, descriptor, ev.Kind) {
		// A hooks delivery of an event this descriptor declares api-owned, for a
		// runner with a live api connection right now: pumpAPIConn (apiconn.go)
		// is already forwarding exactly this event kind from that connection's
		// own driver — TransportFor is how it decides which kinds are its to
		// forward. This hooks copy is the mirror problem: every api-transport
		// spawn ALSO forks a real, hooks-wired companion PTY on the SAME session
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
		// api-owned event" from "the api-transport delivery of that SAME event,
		// arriving through this exact call" — pumpAPIConn's own IngestHook calls
		// satisfy TransportFor==api and HasLiveAPIConnection==true just as
		// thoroughly as the companion PTY's hooks copy does, so an unmarked call
		// used to drop BOTH: session_start through turn_stop reported successful
		// ingestion while the ledger never gained a single turn — confirmed live,
		// the "Codex still not worky" bug.
		slog.DebugContext(ctx, "agent: ingest hook: dropping a hooks-delivered copy of an api-owned event",
			"event", ev.Kind, "runner_id", runnerID, "provider", runner.ProviderID)
		return nil
	}

	if namesAnotherConversation(runner, ev) {
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

// apiOwnsThisEvent reports whether THIS CALL is a redundant hooks-delivered
// echo of an event the api connection already reports — never true for the
// api-transport delivery itself (inflight.FromAPITransport), since canonical
// being api-owned and a live connection existing are both true for that call
// as well; only the ORIGIN of this specific delivery tells the two apart. See
// the call site's own comment for the full mechanism and the bug an unmarked
// check caused.
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
// carry.
func (t *Turns) apiOwnsThisEvent(ctx context.Context, runnerID string, descriptor engineagents.Agent, canonical string) bool {
	if inflight.FromAPITransport(ctx) {
		return false
	}
	return descriptor.TransportFor(canonical) == "api" &&
		t.runners.HasLiveAPIConnection(runnerID) &&
		t.runners.HasDispatchedOverAPI(runnerID)
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

// namesAnotherConversation reports whether ev describes a conversation other
// than the one this runner is on — in which case it is not this chat's to
// record, whatever wire carried it here.
//
// A CONNECTION IS NOT A CONVERSATION. Confirmed live (codex-cli 0.149.1): codex
// pushes a child thread's COMPLETE, independent turn/started..item/*..
// turn/completed cycle down the SAME websocket the runner's own thread uses,
// having never been asked to open it — for a collab agent, and for the review,
// compaction and memory-consolidation threads it spawns on its own. Measured on
// a security review that delegated to a sub-agent: the child's turn/completed
// landed 83 SECONDS before the user's turn actually ended, and closeTurnFromStop
// filed it against the user's chat — StopTurn, Working=false, spinner dark,
// while codex was still writing the answer. The child's assistant messages were
// recorded into the user's transcript on the way past, and its
// thread/status/changed(idle) armed the 5s provider-idle fuse under that same
// live turn.
//
// inbound.Parse skips its own ownership guard for api-transport events, on the
// reasoning that the socket "IS the scoping". It is not. This is the check that
// actually scopes them, and it needs no provider vocabulary to do it: the
// descriptor already maps session_id for exactly these events, so all that was
// missing was the identity comparison.
//
// session_start is exempt: it is the one event that legitimately announces a
// session this runner does not have yet, and move.Decide already arbitrates
// whether that binds, moves or is ignored.
func namesAnotherConversation(
	runner engineagents.Runner,
	ev engineagents.CanonicalEvent,
) bool {
	if ev.Kind == engineagents.HookSessionStart {
		return false
	}
	if ev.SessionID == "" || runner.CurrentSession == "" {
		return false
	}
	return ev.SessionID != runner.CurrentSession
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
