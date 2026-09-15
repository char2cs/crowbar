package turn

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// WaitingForTurnLog is logged the instant a provider switch parks on an in-flight turn.
// It is the only visible sign of a switch that is taking a while, and the user is looking
// at a spinner while it happens — so it is an INFO line, not a debug one.
//
// It is exported because it is the one CAUSAL signal a test of the switch can
// block on: the property under test there is a negative — "the outgoing CLI is
// not killed while the turn is still running" — and a negative can only be proven
// against a moment the test knows the switch has actually reached.
const WaitingForTurnLog = "agent: switch provider: the chat is mid-turn; waiting for the CLI to finish before quitting it"

func (t *Turns) openAssistantTurn(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
) {
	// A new turn is starting, so any "the provider says it is idle" report left
	// over from the previous one is stale — see idle.go.
	t.idle.clear(chat.ID)
	if err := t.activity.OpenTurn(ctx, agentactivity.TurnInput{
		ChatID:     chat.ID,
		TurnID:     openTurnID(chat.ID, runner.ID),
		ProviderID: runner.ProviderID,
		RunnerID:   runner.ID,
		SessionID:  runner.CurrentSession,
		Now:        time.Now(),
	}); err != nil {
		slog.WarnContext(ctx, "agent: ingest hook: open assistant turn",
			"chat_id", chat.ID, "runner_id", runner.ID, "err", err)
	}
}

func openTurnID(chatID, runnerID string) string {
	return "open-" + chatID + "-" + runnerID
}

func (t *Turns) handleTurn(
	ctx context.Context,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
) error {
	chat, ok, err := t.chatForRunner(ctx, runner)
	if err != nil || !ok {
		return err
	}

	switch ev.Kind {
	case "user_prompt":
		return t.openTurnFromPrompt(ctx, chat, runner, agent, ev)
	case "turn_stop":
		return t.closeTurnFromStop(ctx, chat, runner, agent, ev)
	case engineagents.HookTurnFailed:
		return t.closeTurnFromFailure(ctx, chat, runner, ev)
	}
	return nil
}

// openTurnFromPrompt and its recordUserTurn tail live in turn_open.go —
// split out purely to keep this file under the repo's per-file line ceiling;
// see that file for the actual "whose words are these" classification logic.

func (t *Turns) closeTurnFromStop(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
) error {
	// A turn_stop whose turn id is the one compact_pre armed is codex's own
	// compact_start wrapper closing, not an assistant reply — see
	// compaction.go. Nothing below has anything to do: closeAssistantTurn
	// would find an empty message and no stream to close, but StopTurn would
	// still append a real (if inert) turn_stopped event and reset
	// CurrentTurnStarted on a chat that was never marked working for this,
	// on every compaction, forever.
	if t.compacting.consume(chat.ID, ev.TurnID) {
		// The idle report riding this stop is the COMPACTION's, not the assistant
		// turn's — codex sends one with every turn/completed, measured live. Left
		// armed it is a 5s fuse under a turn that is still running, and this path
		// returns before closeAssistantTurn's own clear.
		t.idle.clear(chat.ID)
		return nil
	}
	// THE ANSWER IS DURABLE BEFORE ANYBODY IS TOLD THE TURN ENDED. StopTurn's
	// projection broadcasts Working=false, and the React chat treats that edge as
	// its cue to do ONE ledger read and then stop polling (spec §6). Publishing the
	// state change first raced this append: the read could be served before the
	// assistant row existed, and with the turn over and the queue empty nothing ever
	// re-read it — the reply sat in the ledger, invisible, until an unrelated
	// refresh (a chat switch, a reload) happened to fire. Observed live 2026-08-16.
	//
	// Ordering this way costs nothing the old comment worried about: an empty
	// message is a ledger no-op here and StopTurn below still runs, and a FAILED
	// append still falls through to StopTurn rather than returning early, so the
	// turn state is never left open on a write error.
	appendErr := t.closeAssistantTurn(ctx, chat, runner, ev)
	// Released only ONCE THE LEDGER HAS THE ANSWER: a switch waiting on this turn
	// reads the ledger the moment it wakes, to assemble the handoff. Waking it
	// earlier would hand the incoming CLI a conversation missing the very turn the
	// switch waited for. Deferred so a failed StopTurn still releases the waiter —
	// the turn is over either way, and a switch parked on it would never wake.
	defer t.turns.Complete(runner.ID)
	// The turn ended — which is NOT the same fact as the agent being done, so this
	// carries the CLI's own count of what it left running (ev.AsyncWork) and lets the
	// aggregate fold Working from both. A CLI that hands work to a background task
	// ends its turn right here and goes quiet until that work reports back; clearing
	// Working on the strength of this hook alone is what darkened the spinner under a
	// live subagent. A provider that reports no such level sends 0 and gets exactly
	// the turn-only behaviour it had before — UNLESS Crowbar's own tracking (the same
	// tool/subagent open-close pairing OpenWork already answers termwait's idle check
	// from) can already see the work that CLI never restates. This is only ever
	// consulted at zero, so a provider that DOES restate a level is never second-guessed
	// by it.
	asyncWork := t.fallbackAsyncWork(ctx, chat.ID, ev.AsyncWork)
	stopped, err := t.chats.StopTurn(ctx, chat.ID, time.Now(), asyncWork)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: stop turn: %w", err)
	}
	t.work.Set(chat.ID, stopped.Working)
	if err := t.runners.ReconcilePendingPromptFromLedger(ctx, chat); err != nil {
		slog.WarnContext(ctx, "agent: reconcile React prompt acceptance on turn stop",
			"chat_id", chat.ID, "runner_id", runner.ID, "err", err)
	}
	return appendErr
}

// fallbackAsyncWork returns reported unchanged unless it is zero, in which case OpenWork
// (a tool call or subagent this chat already opened and has not yet closed) stands in for
// a self-reported level the provider never sent.
func (t *Turns) fallbackAsyncWork(ctx context.Context, chatID string, reported int) int {
	if reported != 0 {
		return reported
	}
	open, err := t.OpenWork(ctx, chatID)
	if err != nil {
		slog.WarnContext(ctx, "agent: ingest hook: check open work", "chat_id", chatID, "err", err)
		return reported
	}
	if open {
		return 1
	}
	return reported
}

// restateAsyncWork re-asks StopTurn to fold Working from the CURRENT open-work level —
// the other half of closeTurnFromStop's OpenWork fallback. A CLI that never restates its
// own outstanding count leaves nothing to tell Crowbar when the LAST tracked tool call or
// subagent finally closes, so without this the chat would stay lit until the next turn
// happened to start and stop. It is a no-op while a turn is genuinely open (that turn's
// own eventual turn_stop is what restates it) and a no-op when the level hasn't actually
// changed, so this never appends a redundant event on the hot path (every tool_pre/post).
// Both of this function's preconditions — that no turn is currently open, and that
// the level actually changed — used to be decided HERE, off domain.Chat read back
// through GetChat. That read model is folded by an ASYNCHRONOUS projection, so a
// turn_stop already durable in the log could still read as open: this took the
// early return, no recount was ever appended, and for codex — which reports no
// async-work level of its own — nothing else would ever darken the spinner. The
// chat stayed lit until the next turn happened to start and stop.
//
// Both now live in the StopTurn command's Validate, where asynx evaluates them
// against the authoritative fold and appends at that same version. ErrValidation
// is therefore the ORDINARY no-op answer here, not a failure: a turn is open, or
// the level already stands.
func (t *Turns) restateAsyncWork(ctx context.Context, chatID string) {
	open, err := t.OpenWork(ctx, chatID)
	if err != nil {
		slog.WarnContext(ctx, "agent: restate async work: open work", "chat_id", chatID, "err", err)
		return
	}
	level := 0
	if open {
		level = 1
	}
	stopped, err := t.chats.RestateAsyncWork(ctx, chatID, time.Now(), level)
	if err != nil {
		if !errors.Is(err, asynxModels.ErrValidation) {
			slog.WarnContext(ctx, "agent: restate async work: stop turn", "chat_id", chatID, "err", err)
		}
		return
	}
	t.work.Set(chatID, stopped.Working)
}

func (t *Turns) AwaitTurnComplete(
	ctx context.Context,
	chatID string,
) error {
	logged := false
	for {
		// Read the runner-scoped turn first. StopTurn publishes its authoritative
		// Working result before completing this registry entry, so an async-work
		// handoff can never appear as the forbidden (no turn, idle) combination.
		turnOpen, turnChanged := t.turns.Watch(chatID)
		working, known, workChanged := t.work.Observe(chatID)
		if !known {
			var err error
			if working, workChanged, err = t.seedWorkFromProjection(ctx, chatID); err != nil {
				return err
			}
		}
		if !turnOpen && !working {
			return nil
		}

		if !logged {
			slog.InfoContext(ctx, WaitingForTurnLog, "chat_id", chatID)
			logged = true
		}
		select {
		case <-turnChanged:
		case <-workChanged:
		case <-ctx.Done():
			return fmt.Errorf("agent: switch provider: waiting for the chat to become idle: %w", ctx.Err())
		}
	}
}

func (t *Turns) ChatWorking(ctx context.Context, chatID string) (bool, error) {
	if working, known, _ := t.work.Observe(chatID); known {
		return working, nil
	}
	chat, err := t.chats.GetChat(ctx, chatID)
	if err != nil {
		return false, err
	}
	if working, known, _ := t.work.Observe(chatID); known {
		return working, nil
	}
	return chat.Working, nil
}

// RecordStop notes that a person cut chatID's in-flight turn short — the
// counterpart of compaction's HookCompactPre/Post pair (observation.go), but
// Crowbar's own doing rather than a translated provider hook: nothing on the
// wire announces "a human clicked Stop", so this is the one place that fact
// can be recorded at all. Opened and resolved in the same call, back to back,
// because unlike compaction there is no later event to close it on.
//
// CALL THIS ONLY ONCE THE STOP HAS ACTUALLY TAKEN EFFECT — after retire's kill
// or after an interruptTurn send that itself blocked until the CLI's turn
// genuinely ended, never at the moment Stop was merely clicked. Crowbar does
// not learn the full story until the CLI has actually stopped; recording it
// earlier durably marks the turn "Interrupted" while the CLI is still
// generating, both in the wrong ledger position (ahead of whatever real
// content the CLI still produces) and factually wrong (nothing has stopped
// yet). Confirmed live: a stop that only asked a live codex connection to
// cancel, without waiting for it to actually do so, left the CLI streaming
// real tool calls and assistant text for another full minute after the
// "Interrupted" divider had already rendered.
//
// A no-op when the chat is idle: StopChat is also what closing a chat TAB
// calls, and quitting an already-quiet CLI is not an interruption of anything.
//
// Takes runnerID's own hook gate — the SAME one IngestHookDelivery holds
// across its whole ingest — before touching the activity ledger. Without it,
// this could commit its Interrupt in the gap between a hook for this exact
// runner being admitted and its effects landing: interruptTurn's Send only
// waits for the API connection to say the turn is over, which says nothing
// about whether that turn's OWN closing/reopening hook deliveries have
// finished being ingested yet. Confirmed live: an Interrupted divider
// anchored to a message's turn that a self-continuation hook had already
// superseded milliseconds earlier, landing ahead of replies the CLI had
// already produced by the time Stop was clicked.
func (t *Turns) RecordStop(ctx context.Context, chatID, runnerID string) error {
	defer t.hookGates.Lock(runnerID)()
	if len(t.turns.Inflight(chatID)) == 0 {
		return nil
	}
	now := time.Now()
	id := "interrupt-" + fallbackID()
	if err := t.activity.Interrupt(
		ctx, chatID, id, engineagents.InterruptStopped, "", now,
	); err != nil {
		return fmt.Errorf("agent: record stop: interrupt: %w", err)
	}
	if err := t.activity.ResolveInterruption(
		ctx, chatID, id, engineagents.InterruptStopped, "", now,
	); err != nil {
		return fmt.Errorf("agent: record stop: resolve interruption: %w", err)
	}
	return nil
}

// RecordChatSwitch notes, durably, that Crowbar itself changed chatID's
// provider, model or effort — Crowbar's own doing, the same as RecordStop,
// never something a provider hook reports. kind is one of
// InterruptProviderSwitched/InterruptModelChanged/InterruptEffortChanged;
// detail is the new value. Opened and resolved in the same call, back to
// back, exactly like RecordStop: Crowbar already knows the full story the
// instant it decides to make the change, so there is no later event to
// close it on. The caller decides whether the value actually changed —
// this method has no "old" to compare against, so it always records.
func (t *Turns) RecordChatSwitch(ctx context.Context, chatID, kind, detail string) error {
	now := time.Now()
	id := "interrupt-" + fallbackID()
	if err := t.activity.Interrupt(ctx, chatID, id, kind, detail, now); err != nil {
		return fmt.Errorf("agent: record chat switch: interrupt: %w", err)
	}
	if err := t.activity.ResolveInterruption(ctx, chatID, id, kind, detail, now); err != nil {
		return fmt.Errorf("agent: record chat switch: resolve interruption: %w", err)
	}
	return nil
}

func deriveTitle(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		r := []rune(line)
		if len(r) > 60 {
			return strings.TrimSpace(string(r[:60])) + "…"
		}
		return line
	}
	return ""
}

func (t *Turns) seedWorkFromProjection(
	ctx context.Context,
	chatID string,
) (bool, <-chan struct{}, error) {
	chat, err := t.chats.GetChat(ctx, chatID)
	if err != nil {
		return false, nil, fmt.Errorf("agent: switch provider: inspect chat work: %w", err)
	}
	current, known, changed := t.work.Observe(chatID)
	if known {
		return current, changed, nil
	}
	return chat.Working, changed, nil
}
