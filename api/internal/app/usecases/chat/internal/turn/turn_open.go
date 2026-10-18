package turn

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/promptsigil"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// turnHold is a runner held against its end. startTurn needs one, and only
// openingTurn makes one, so no turn opens on a runner whose end is under way.
type turnHold struct{ runner engineagents.Runner }

// openingTurn runs open on runnerID's runner held against its end, and reports
// false without running it once that end is under way: nothing would close a
// turn opened after it.
func (t *Turns) openingTurn(runnerID string, open func(turnHold) error) (bool, error) {
	return t.runners.TurnRunner(runnerID, func(runner engineagents.Runner) error {
		return open(turnHold{runner: runner})
	})
}

// startTurn marks h's chat working and its turn in flight, the same fact
// without the read model's lag: a provider switch blocks on the latter
// (inflight.Turns) so it never quits a CLI that is still answering.
func (t *Turns) startTurn(ctx context.Context, h turnHold) error {
	chatID := h.runner.CurrentChatID
	started, err := t.chats.StartTurn(ctx, chatID, time.Now())
	if err != nil {
		return fmt.Errorf("agent: ingest hook: start turn: %w", err)
	}
	t.work.Set(chatID, started.Working)
	t.turns.Begin(ctx, h.runner.ID, chatID)
	return nil
}

func (t *Turns) openTurnFromPrompt(
	ctx context.Context,
	chat domain.Chat,
	h turnHold,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
) error {
	runner := h.runner
	// A prompt fired inside a subagent is the parent agent talking to it, never the
	// user; the subagent's own events already carry its activity.
	if ev.Subagent != nil {
		return nil
	}
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
			if err := t.startTurn(ctx, h); err != nil {
				return err
			}
			t.openAssistantTurn(ctx, chat, runner)
			return nil
		}
		// A real prompt rode alongside the injected context in this same
		// spawn: record and confirm ONLY the user's own words, exactly like
		// the ordinary path below, never Crowbar's own injected preamble.
		rest := ev
		rest.Message = remainder
		return t.recordUserTurn(ctx, chat, h, agent, rest)
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
	// about to work on this — and a pending Crowbar prompt stays pending, because
	// nothing Crowbar queued was accepted here.
	if injected, ok := engineagents.MatchInjectedPrompt(agent, ev.Message); ok {
		slog.DebugContext(ctx, "agent: ingest hook: user_prompt was injected by the provider's harness",
			"chat_id", chat.ID, "runner_id", runner.ID, "provider", runner.ProviderID,
			"kind", injected.Kind, "needle", injected.Needle)
		if err := t.recordOtherTurn(ctx, chat.ID, runner, ev.TurnID, ledger.OriginHarness, ev.Message); err != nil {
			return err
		}
		if err := t.startTurn(ctx, h); err != nil {
			return err
		}
		appendErr := t.conversations.AppendRunnerTurn(
			ctx, chat, runner.ProviderID, runner.ID, t.boundSession(ctx, chat.ID, runner.ProviderID),
			domain.TurnRoleHarness, ev.Message,
		)
		t.openAssistantTurn(ctx, chat, runner)
		return appendErr
	}
	return t.recordUserTurn(ctx, chat, h, agent, ev)
}

// recordUserTurn is openTurnFromPrompt's shared tail for text genuinely
// authored by the user — whether that is the WHOLE hook payload (the
// ordinary case) or the remainder ConsumeInjectedPrefix handed back after
// stripping Crowbar's own injected preamble off the front of it.
func (t *Turns) recordUserTurn(
	ctx context.Context,
	chat domain.Chat,
	h turnHold,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
) error {
	runner := h.runner
	rawText := ev.Message
	// This hook's own message IS the text the CLI actually received — which,
	// whenever Crowbar's own dispatch referenced an attachment, is the file's
	// real absolute path in the durable store, not the logical
	// chats/<chatID>/attachments/<file> reference every other consumer of a
	// stored turn (the asset-serving endpoint included) expects. Restore it
	// before this text becomes anything durable: the title derived from it and
	// the rows below, whose text must equal the prompt Crowbar recorded. A
	// prompt typed straight into the CLI's own terminal never contains this
	// shape, so the rewrite is a no-op for it.
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
	// A user prompt opens the turn: the read model (and the workspace spinner)
	// see a live turn.
	if err := t.startTurn(ctx, h); err != nil {
		return err
	}
	// The hook is the provider's acknowledgement of the prompt Crowbar recorded,
	// matched on the text the request carried before dispatch rewrote it.
	requestID, acceptErr := t.acceptPrompt(ctx, chat.ID, runner, ev.TurnID, userText)
	// Named by its request, the row is how the client that queued this prompt
	// recognises it.
	userCtx := ctx
	if requestID != "" {
		userCtx = inflight.WithRecordID(ctx, requestID)
	}
	appendErr := t.conversations.AppendRunnerTurn(
		userCtx, chat, runner.ProviderID, runner.ID, t.boundSession(ctx, chat.ID, runner.ProviderID),
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
	return acceptErr
}

// acceptPrompt starts the turn in the ledger: for the oldest pending Crowbar
// prompt this runner carries when text is that prompt, else for text typed at
// the CLI. It returns the request id of the prompt accepted, if any.
func (t *Turns) acceptPrompt(
	ctx context.Context, chatID string, runner engineagents.Runner, providerTurnID, text string,
) (string, error) {
	var accepted string
	started := t.turnStarted(ctx, chatID, runner, providerTurnID)
	_, _, err := t.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		for _, p := range st.PendingPrompts() {
			if p.Text != text || (p.RunnerID != "" && p.RunnerID != runner.ID) {
				continue
			}
			accepted = p.RequestID
			body := started
			body.RequestID = p.RequestID
			row, err := ledger.NewRow(ledger.KindTurnStarted, p.TurnID, "", body)
			return []ledger.Row{row}, err
		}
		accepted = ""
		return otherTurnRows(started, ledger.OriginProvider, text)
	})
	if err != nil {
		return "", fmt.Errorf("agent: ingest hook: accept prompt: %w", err)
	}
	return accepted, nil
}

// recordOtherTurn starts a turn nobody sent through Crowbar.
func (t *Turns) recordOtherTurn(
	ctx context.Context, chatID string, runner engineagents.Runner, providerTurnID, origin, text string,
) error {
	started := t.turnStarted(ctx, chatID, runner, providerTurnID)
	if _, _, err := t.ledger.Do(ctx, chatID, func(ledger.State) ([]ledger.Row, error) {
		return otherTurnRows(started, origin, text)
	}); err != nil {
		return fmt.Errorf("agent: ingest hook: record %s turn: %w", origin, err)
	}
	return nil
}

// turnStarted is the turn_started body for a turn runner's CLI took on the
// chat's bound session: the turn that makes that session resumable.
func (t *Turns) turnStarted(
	ctx context.Context, chatID string, runner engineagents.Runner, providerTurnID string,
) ledger.TurnStarted {
	return ledger.TurnStarted{
		ProviderTurnID: providerTurnID, RunnerID: runner.ID,
		Provider: runner.ProviderID, SessionID: t.boundSession(ctx, chatID, runner.ProviderID),
	}
}

func otherTurnRows(started ledger.TurnStarted, origin, text string) ([]ledger.Row, error) {
	turnID := uuid.NewString()
	msg, err := ledger.NewRow(ledger.KindUserMessage, turnID, "", ledger.UserMessage{Text: text, Origin: origin})
	if err != nil {
		return nil, err
	}
	row, err := ledger.NewRow(ledger.KindTurnStarted, turnID, "", started)
	return []ledger.Row{msg, row}, err
}
