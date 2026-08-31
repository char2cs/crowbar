package turn

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

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

func (t *Turns) openTurnFromPrompt(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
) error {
	// WHOSE WORDS ARE THESE? Three different authors reach this one hook — the
	// user, Crowbar's own injected handoff, and the provider's harness — and the
	// two checks below are what tell them apart. Neither is a guess about the
	// runner: a prompt Crowbar delivered arrived in the argv of the process
	// Crowbar spawned, so the only thing left to classify here is content.
	//
	// Crowbar's own context document coming back at us: a provider whose only
	// resume channel is a user message (codex) fires user_prompt with the very
	// handoff we injected. That is not something the user said — recording it would
	// put the handoff in the ledger as a "user" turn, and the NEXT handoff would
	// then quote it inside itself (the nesting seen live). Drop it from the ledger
	// and from title derivation, but still open the turn: the CLI really is working
	// on it, and the workspace's working overlay must say so.
	if t.agents.WasInjected(runner.ID, ev.Message) {
		started, err := t.chats.StartTurn(ctx, chat.ID, time.Now())
		if err != nil {
			return fmt.Errorf("agent: ingest hook: start turn: %w", err)
		}
		t.work.Set(chat.ID, started.Working)
		t.turns.Begin(runner.ID, chat.ID)
		t.openAssistantTurn(ctx, chat, runner)
		return nil
	}
	// The PROVIDER's own harness talking to its own model on the user's hook: a
	// background-subagent completion report is the measured case, and the ledger
	// recorded every one of them as something the user said. It is the sibling of
	// the branch above and deliberately not a copy of it — that one drops the text
	// because Crowbar wrote it and already has it, and this one must NOT, because
	// this text is real context the agent received and its next answer refers to
	// it. Dropped, the reply would have no antecedent; attributed, the user is
	// quoted saying something they never wrote, which is what get_chat_log was
	// serving to other agents. So it is recorded under its own role.
	//
	// No derived title: a chat named after a subagent's completion report is named
	// after nothing its user did. The turn still opens — the agent genuinely is
	// about to work on this — and no prompt-delivery journal is advanced, because
	// nothing Crowbar queued was accepted here.
	if injected, ok := engineagents.MatchInjectedPrompt(agent, ev.Message); ok {
		slog.DebugContext(ctx, "agent: ingest hook: user_prompt was injected by the provider's harness",
			"chat_id", chat.ID, "runner_id", runner.ID, "provider", runner.ProviderID,
			"kind", injected.Kind, "needle", injected.Needle)
		started, err := t.chats.StartTurn(ctx, chat.ID, time.Now())
		if err != nil {
			return fmt.Errorf("agent: ingest hook: start turn: %w", err)
		}
		t.work.Set(chat.ID, started.Working)
		t.turns.Begin(runner.ID, chat.ID)
		appendErr := t.conversations.AppendRunnerTurn(
			ctx, chat, runner.ProviderID, runner.ID, runner.CurrentSession,
			domain.TurnRoleHarness, ev.Message,
		)
		t.openAssistantTurn(ctx, chat, runner)
		return appendErr
	}
	if err := t.conversations.RenameChat(ctx, chat.ID, deriveTitle(ev.Message), "derived"); err != nil {
		slog.WarnContext(ctx, "agent: ingest hook: derived title", "err", err, "chat_id", chat.ID)
	}
	// A user prompt opens the turn: mark the chat Working so the read model (and
	// the workspace spinner) see a live turn.
	started, err := t.chats.StartTurn(ctx, chat.ID, time.Now())
	if err != nil {
		return fmt.Errorf("agent: ingest hook: start turn: %w", err)
	}
	t.work.Set(chat.ID, started.Working)
	// And record it as IN FLIGHT, which is the same fact without the read model's lag
	// in front of it — a provider switch blocks on this rather than on Working, so that
	// it never quits a CLI that is still answering (inflight.Turns).
	t.turns.Begin(runner.ID, chat.ID)
	appendErr := t.conversations.AppendRunnerTurn(
		ctx, chat, runner.ProviderID, runner.ID, runner.CurrentSession,
		domain.TurnRoleUser, ev.Message,
	)
	// The reply this prompt is about to produce, opened NOW so the tool calls,
	// subagents and interruptions that follow attach to it. Without an open turn
	// each of them would open one of its own, and the reply recorded at turn_stop
	// would be a separate record — leaving the UI unable to say which activity
	// produced which answer.
	t.openAssistantTurn(ctx, chat, runner)
	// The hook is the provider's acknowledgement that the argv prompt was
	// accepted. Advance the journal even when the ledger write failed: the hook
	// itself is positive delivery evidence, and leaving the request spawned
	// would wedge every future prompt. Conversely, a journal failure after a
	// successful ledger append is repaired from that attributed turn by the
	// turn_stop and pre-destructive reconciliation paths.
	confirmErr := t.runners.ConfirmPromptAccepted(ctx, chat, runner, ev.Message)
	if appendErr != nil {
		return appendErr
	}
	if confirmErr != nil {
		return fmt.Errorf("agent: confirm React prompt acceptance: %w", confirmErr)
	}
	return nil
}

func (t *Turns) closeTurnFromStop(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
) error {
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
	// the turn-only behaviour it had before.
	stopped, err := t.chats.StopTurn(ctx, chat.ID, time.Now(), ev.AsyncWork)
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
// because unlike compaction there is no later event to close it on — Crowbar
// already knows the full story the instant it decides to stop the CLI.
//
// A no-op when the chat is idle: StopChat is also what closing a chat TAB
// calls, and quitting an already-quiet CLI is not an interruption of anything.
func (t *Turns) RecordStop(ctx context.Context, chatID string) error {
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
