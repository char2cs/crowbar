package turn

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/promptsigil"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

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
	// handoff we injected — or, whenever a switch resumed an already-turned
	// session while ALSO delivering a real prompt, with that handoff merged
	// directly ahead of the real message in the SAME positional argv token
	// (mergeLeadingPositional, spawnsteps.go — a single-positional CLI cannot
	// receive them as two separate ones; see that function's own doc). The
	// injected part is never something the user said — recording it would put
	// the handoff in the ledger as a "user" turn, and the NEXT handoff would
	// then quote it inside itself (the nesting seen live). ConsumeInjectedPrefix
	// strips exactly that part and hands back whatever real text remains.
	if remainder, injected := t.agents.ConsumeInjectedPrefix(runner.ID, ev.Message); injected {
		if remainder == "" {
			// A bare echo — nothing else was riding this spawn. Drop it from
			// the ledger and from title derivation, but still open the turn:
			// the CLI really is working on it, and the workspace's working
			// overlay must say so.
			started, err := t.chats.StartTurn(ctx, chat.ID, time.Now())
			if err != nil {
				return fmt.Errorf("agent: ingest hook: start turn: %w", err)
			}
			t.work.Set(chat.ID, started.Working)
			t.turns.Begin(runner.ID, chat.ID)
			t.openAssistantTurn(ctx, chat, runner)
			return nil
		}
		// A real prompt rode alongside the injected context in this same
		// spawn: record and confirm ONLY the user's own words, exactly like
		// the ordinary path below, never Crowbar's own injected preamble.
		return t.recordUserTurn(ctx, chat, runner, agent, remainder)
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
	return t.recordUserTurn(ctx, chat, runner, agent, ev.Message)
}

// recordUserTurn is openTurnFromPrompt's shared tail for text genuinely
// authored by the user — whether that is the WHOLE hook payload (the
// ordinary case) or the remainder ConsumeInjectedPrefix handed back after
// stripping Crowbar's own injected preamble off the front of it.
func (t *Turns) recordUserTurn(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	agent engineagents.Agent,
	rawText string,
) error {
	// This hook's own message IS the text the CLI actually received — which,
	// whenever Crowbar's own dispatch referenced an attachment, is the file's
	// real absolute path in the durable store, not the logical
	// chats/<chatID>/attachments/<file> reference every other consumer of a
	// stored turn (the asset-serving endpoint included) expects. Restore it
	// before this text becomes anything durable: the title derived from it,
	// the ledger row itself below, and the accept-confirmation hash beneath
	// that. A prompt typed straight into the CLI's own terminal never
	// contains this shape, so the rewrite is a no-op for it.
	userText := rawText
	if chatsDir, cErr := t.ws.AgentChatsDir(ctx, runner.WorkspaceID); cErr != nil {
		slog.WarnContext(ctx, "agent: ingest hook: resolve chats dir for attachment ref restore",
			"chat_id", chat.ID, "runner_id", runner.ID, "err", cErr)
	} else {
		userText = worktreepath.RestoreDurableAttachmentRefs(rawText, chatsDir, chat.ID)
	}
	// And the other thing dispatch may have added in front of it: the escape
	// that stops Crowbar's own `![alt](…)` from reading as this CLI's shell-mode
	// sigil (promptsigil.Guard, spawn.go). The person did not type it, so it has
	// no business in the turn this stores forever.
	sigils, escape := agent.PromptLeadingSigils()
	userText = promptsigil.Strip(sigils, escape, chat.ID, userText)
	if err := t.conversations.RenameChat(ctx, chat.ID, deriveTitle(userText), "derived"); err != nil {
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
	// The hook is the provider's acknowledgement that the argv prompt was
	// accepted. userText, not rawText: ConfirmPromptAccepted hashes this
	// against the journal's own hash of the ORIGINAL dispatch text (prompts.go's
	// Begin call, taken straight from the request before Guard/materialize or
	// any injected preamble ever touched it) — the restored, stripped, and
	// (for the merged case) preamble-free text is what can actually match it.
	// The ledger is written whether or not the journal was: the hook itself is
	// positive delivery evidence. A journal failure is repaired from the
	// attributed turn by the turn_stop and pre-destructive reconciliation paths.
	requestID, confirmErr := t.runners.ConfirmPromptAccepted(ctx, chat, runner, userText)
	// Named by its request, the row is how the client that queued this prompt
	// recognises it.
	userCtx := ctx
	if requestID != "" {
		userCtx = inflight.WithRecordID(ctx, requestID)
	}
	appendErr := t.conversations.AppendRunnerTurn(
		userCtx, chat, runner.ProviderID, runner.ID, runner.CurrentSession,
		domain.TurnRoleUser, userText,
	)
	// The reply this prompt is about to produce, opened NOW so the tool calls,
	// subagents and interruptions that follow attach to it. Without an open turn
	// each of them would open one of its own, and the reply recorded at turn_stop
	// would be a separate record — leaving the UI unable to say which activity
	// produced which answer.
	t.openAssistantTurn(ctx, chat, runner)
	if appendErr != nil {
		return appendErr
	}
	if confirmErr != nil {
		return fmt.Errorf("agent: confirm React prompt acceptance: %w", confirmErr)
	}
	return nil
}
