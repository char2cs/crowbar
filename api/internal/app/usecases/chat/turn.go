package chat

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

func (u *turnUsecase) openAssistantTurn(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
) {
	if err := u.activity.OpenTurn(ctx, agentactivity.TurnInput{
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

func (u *turnUsecase) handleTurn(
	ctx context.Context,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
) error {
	chat, ok, err := u.chatForRunner(ctx, runner)
	if err != nil || !ok {
		return err
	}

	switch ev.Kind {
	case "user_prompt":
		return u.openTurnFromPrompt(ctx, chat, runner, agent, ev)
	case "turn_stop":
		return u.closeTurnFromStop(ctx, chat, runner, agent, ev)
	case engineagents.HookTurnFailed:
		return u.closeTurnFromFailure(ctx, chat, runner, ev)
	}
	return nil
}

func (u *turnUsecase) openTurnFromPrompt(
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
	if u.agents.WasInjected(runner.ID, ev.Message) {
		started, err := u.chats.StartTurn(ctx, chat.ID, time.Now())
		if err != nil {
			return fmt.Errorf("agent: ingest hook: start turn: %w", err)
		}
		u.work.set(chat.ID, started.Working)
		u.turns.begin(runner.ID, chat.ID)
		u.openAssistantTurn(ctx, chat, runner)
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
		started, err := u.chats.StartTurn(ctx, chat.ID, time.Now())
		if err != nil {
			return fmt.Errorf("agent: ingest hook: start turn: %w", err)
		}
		u.work.set(chat.ID, started.Working)
		u.turns.begin(runner.ID, chat.ID)
		appendErr := u.chat.appendRunnerTurn(
			ctx, chat, runner.ProviderID, runner.ID, runner.CurrentSession,
			domain.TurnRoleHarness, ev.Message,
		)
		u.openAssistantTurn(ctx, chat, runner)
		return appendErr
	}
	if err := u.chat.RenameChat(ctx, chat.ID, deriveTitle(ev.Message), "derived"); err != nil {
		slog.WarnContext(ctx, "agent: ingest hook: derived title", "err", err, "chat_id", chat.ID)
	}
	// A user prompt opens the turn: mark the chat Working so the read model (and
	// the workspace spinner) see a live turn.
	started, err := u.chats.StartTurn(ctx, chat.ID, time.Now())
	if err != nil {
		return fmt.Errorf("agent: ingest hook: start turn: %w", err)
	}
	u.work.set(chat.ID, started.Working)
	// And record it as IN FLIGHT, which is the same fact without the read model's lag
	// in front of it — a provider switch blocks on this rather than on Working, so that
	// it never quits a CLI that is still answering (turnWaits).
	u.turns.begin(runner.ID, chat.ID)
	appendErr := u.chat.appendRunnerTurn(
		ctx, chat, runner.ProviderID, runner.ID, runner.CurrentSession,
		domain.TurnRoleUser, ev.Message,
	)
	// The reply this prompt is about to produce, opened NOW so the tool calls,
	// subagents and interruptions that follow attach to it. Without an open turn
	// each of them would open one of its own, and the reply recorded at turn_stop
	// would be a separate record — leaving the UI unable to say which activity
	// produced which answer.
	u.openAssistantTurn(ctx, chat, runner)
	// The hook is the provider's acknowledgement that the argv prompt was
	// accepted. Advance the journal even when the ledger write failed: the hook
	// itself is positive delivery evidence, and leaving the request spawned
	// would wedge every future prompt. Conversely, a journal failure after a
	// successful ledger append is repaired from that attributed turn by the
	// turn_stop and pre-destructive reconciliation paths.
	confirmErr := u.runner.confirmPromptAccepted(ctx, chat, runner, ev.Message)
	if appendErr != nil {
		return appendErr
	}
	if confirmErr != nil {
		return fmt.Errorf("agent: confirm React prompt acceptance: %w", confirmErr)
	}
	return nil
}

func (u *turnUsecase) closeTurnFromStop(
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
	appendErr := u.closeAssistantTurn(ctx, chat, runner, ev)
	// Released only ONCE THE LEDGER HAS THE ANSWER: a switch waiting on this turn
	// reads the ledger the moment it wakes, to assemble the handoff. Waking it
	// earlier would hand the incoming CLI a conversation missing the very turn the
	// switch waited for. Deferred so a failed StopTurn still releases the waiter —
	// the turn is over either way, and a switch parked on it would never wake.
	defer u.turns.complete(runner.ID)
	// The turn ended — which is NOT the same fact as the agent being done, so this
	// carries the CLI's own count of what it left running (ev.AsyncWork) and lets the
	// aggregate fold Working from both. A CLI that hands work to a background task
	// ends its turn right here and goes quiet until that work reports back; clearing
	// Working on the strength of this hook alone is what darkened the spinner under a
	// live subagent. A provider that reports no such level sends 0 and gets exactly
	// the turn-only behaviour it had before.
	stopped, err := u.chats.StopTurn(ctx, chat.ID, time.Now(), ev.AsyncWork)
	if err != nil {
		return fmt.Errorf("agent: ingest hook: stop turn: %w", err)
	}
	u.work.set(chat.ID, stopped.Working)
	if err := u.runner.reconcilePendingPromptFromLedger(ctx, chat); err != nil {
		slog.WarnContext(ctx, "agent: reconcile React prompt acceptance on turn stop",
			"chat_id", chat.ID, "runner_id", runner.ID, "err", err)
	}
	return appendErr
}

func (u *turnUsecase) awaitTurnComplete(
	ctx context.Context,
	chatID string,
) error {
	logged := false
	for {
		// Read the runner-scoped turn first. StopTurn publishes its authoritative
		// Working result before completing this registry entry, so an async-work
		// handoff can never appear as the forbidden (no turn, idle) combination.
		turnOpen, turnChanged := u.turns.watch(chatID)
		working, known, workChanged := u.work.observe(chatID)
		if !known {
			var err error
			if working, workChanged, err = u.seedWorkFromProjection(ctx, chatID); err != nil {
				return err
			}
		}
		if !turnOpen && !working {
			return nil
		}

		if !logged {
			slog.InfoContext(ctx, waitingForTurnLog, "chat_id", chatID)
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

func (u *turnUsecase) chatWorking(ctx context.Context, chatID string) (bool, error) {
	if working, known, _ := u.work.observe(chatID); known {
		return working, nil
	}
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return false, err
	}
	if working, known, _ := u.work.observe(chatID); known {
		return working, nil
	}
	return chat.Working, nil
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

func (u *turnUsecase) seedWorkFromProjection(
	ctx context.Context,
	chatID string,
) (bool, <-chan struct{}, error) {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return false, nil, fmt.Errorf("agent: switch provider: inspect chat work: %w", err)
	}
	current, known, changed := u.work.observe(chatID)
	if known {
		return current, changed, nil
	}
	return chat.Working, changed, nil
}
