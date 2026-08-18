package agent

import (
	"context"
	"errors"
	"log/slog"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// closeStalledTurn ends a turn a provider abandoned WITHOUT EVER SAYING SO, and
// tells the chat, in the provider's own words, why.
//
// It is the third and last way a turn can be closed, and it exists because the
// other two cannot reach this case. A turn normally ends on the provider's own
// terminating hook. When the CLI dies instead, the runner-exit reconcile closes
// it (reconcileRunnerExit → closeAbandonedTurn). The measured failure is neither:
// codex-cli 0.146.0 out of quota accepted the prompt, fired user_prompt, painted
// its usage-limit banner, and then STAYED ALIVE — no Stop hook, so nothing closed
// the turn, and no exit, so nothing ever would. The chat spun for 44 minutes and
// stopped only because a human switched provider and the displace closed it.
//
// It is called from the terminal sweep, which owns the decision. Every gate is
// there (see the termwait package doc); this function is the ACT, and it makes no
// judgement of its own about whether the CLI is working.
//
// # Why closeAbandonedTurn is not called
//
// It is the same COMMAND, deliberately — AbandonTurn, so the two paths cannot
// drift on what closing a turn means — but it cannot be the same FUNCTION.
// closeAbandonedTurn's first guard is "does a live runner hold this chat?", and
// it goes home when one does, because on its path a live runner means somebody
// else has taken the chat over and the turn is not its to close. Here a live
// runner is GATE ONE: the whole premise is a process that is still up and has
// stopped working. The two functions are answering the same question in
// situations where the same evidence means opposite things.
// # The order of the three writes is the design
//
// THE NOTICE IS DURABLE BEFORE ANYBODY IS TOLD THE TURN ENDED. AbandonTurn's
// projection broadcasts Working=false, and the chat treats that edge as its cue
// to do ONE ledger read and then stop; a ledger append emits no frame of its own.
// Publishing the state change first therefore races the append: the read is
// served before the notice row exists, and with the turn over nothing ever
// re-reads — the explanation sits in the ledger, invisible, until an unrelated
// refresh happens to fire. That is exactly the race handleTurn's turn_stop arm
// documents, observed live on 2026-08-16, and it would have left defect 2
// unfixed underneath a correct fix for defect 1.
//
// What made the naive ordering tempting is a real constraint: the notice must
// never be stamped on a turn that ENDED NORMALLY. Writing it only on
// AbandonTurn's success arm gave that for free, since its ErrValidation is the
// command's authoritative fold saying there was nothing to close.
//
// The constraint is met a different way instead, by asking a different authority.
// u.work is the process-local mirror of the aggregate AS RETURNED BY THE TURN
// COMMANDS THEMSELVES, with no projection in between (see chatWorkStates), so it
// cannot report an open turn that has already ended — which is precisely what
// GetChat's asynchronously folded read model can do, and why the detector's own
// Working read is not good enough to act on here. If it says this chat has no
// open turn, nothing happens at all, and that costs no command dispatch either.
func (u *Usecase) closeStalledTurn(ctx context.Context, stall termwait.Stall) {
	if stall.ChatID == "" {
		return
	}

	// The non-lagging gate. Unknown means this process has issued no turn command
	// for the chat, which for a wedge is impossible — a PTY does not survive a
	// restart, so the runner holding one was started here and its user_prompt hook
	// went through StartTurn — and treating it as "nothing to close" is the safe
	// reading of a state we cannot vouch for either way.
	working, known, _ := u.work.observe(stall.ChatID)
	if !known || !working {
		return
	}

	// The conversation record is closed FIRST, exactly as the reconcile path does
	// it: a turn left open there holds whatever tool calls were in flight, and
	// those render as running for as long as the record says so. It is also what
	// makes the notice below land as a turn of its own rather than as text
	// attached to the turn that just failed.
	if err := u.activity.Abandon(ctx, stall.ChatID, time.Now()); err != nil {
		slog.WarnContext(ctx, "agent: close stalled turn: abandon conversation record",
			"chat_id", stall.ChatID, "err", err)
	}
	u.recordStallNotice(ctx, stall)

	// AbandonTurn, not StopTurn, for the reason closeAbandonedTurn gives: the CLI
	// will never restate the level of async work it last reported, and Working is
	// folded from the turn OR that level — so a plain stop would leave the number
	// standing and the chat spinning on work nothing is doing. That is this same
	// wedge, one field over.
	abandoned, err := u.chats.AbandonTurn(ctx, stall.ChatID, time.Now())
	switch {
	case err == nil:
		u.work.set(stall.ChatID, abandoned.Working)
	case errors.Is(err, asynxModels.ErrValidation):
		// The command's fold says there was nothing to close after all — the turn
		// ended on its own hook in the microseconds since the gate above. Benign,
		// and the residue is one notice row on a chat that finished normally.
		//
		// That residue is the price of the ordering, and it is the right way round:
		// the alternative left the explanation invisible in EVERY wedge, while this
		// costs a spurious row only when a 120-second-quiescent CLI fires a hook
		// inside a window a few instructions wide.
		u.work.set(stall.ChatID, false)
	default:
		slog.WarnContext(ctx, "agent: close stalled turn: abandon turn",
			"chat_id", stall.ChatID, "err", err)
		return
	}

	slog.InfoContext(ctx, "agent: closed a turn its provider abandoned",
		"chat_id", stall.ChatID, "provider", stall.ProviderID,
		"runner_id", stall.RunnerID, "notice", stall.Notice.Kind)
}

// recordStallNotice writes the provider's own sentence into the chat as a turn.
//
// Role `notice`: not the user, not the model, and not the vendor harness — it is
// Crowbar reporting something it observed about the chat. The ROLE carries that
// framing, which is why the TEXT is the provider's words and nothing else. A
// Crowbar preamble glued to the front would be a paraphrase of a sentence that is
// already better than any paraphrase of it: codex's banner names the plan to
// upgrade to, the URL to buy credits at, and the exact time the limit resets.
//
// It goes through recordTurn like every other turn in the ledger, so it is
// idempotent by turn id, attributed to the runner that produced it, and visible
// to every reader that can already read a conversation. A failure to write it is
// logged and swallowed: the turn is already closed and the spinner already
// stopped, and losing the explanation is much better than not closing.
func (u *Usecase) recordStallNotice(ctx context.Context, stall termwait.Stall) {
	if stall.Notice.Text == "" {
		return
	}
	chat, err := u.chats.GetChat(ctx, stall.ChatID)
	if err != nil {
		slog.WarnContext(ctx, "agent: close stalled turn: read chat for notice",
			"chat_id", stall.ChatID, "err", err)
		return
	}
	if err := u.recordTurn(
		ctx, chat,
		stall.ProviderID, stall.RunnerID, stall.SessionID,
		domain.TurnRoleNotice, stall.Notice.Text, "",
	); err != nil {
		slog.WarnContext(ctx, "agent: close stalled turn: record notice",
			"chat_id", stall.ChatID, "err", err)
	}
}

// MatchTerminalNotice is the detector's second provider seam, and the exact
// counterpart of MatchTerminalPrompt: it resolves the descriptor for a provider
// and asks the engine whether this screen carries a message the CLI paints
// instead of finishing a turn.
//
// The needles and the wording both live in the descriptor, so a CLI release that
// repaints its banner is a YAML edit on disk rather than a daemon build — and
// nothing in this package, or above it, learns a provider's vocabulary. What DOES
// travel up is the provider's captured sentence, which is the one thing here that
// is meant to be read by a human.
//
// A failure to resolve is silence, not an error: an unresolvable descriptor
// declares no notices, and a provider that declares none never stalls a chat. The
// same is true of an engine that does not implement the capability at all.
func (u *Usecase) MatchTerminalNotice(
	ctx context.Context,
	providerID string,
	screen string,
) (engineagents.TerminalNotice, bool) {
	home, err := u.home()
	if err != nil {
		return engineagents.TerminalNotice{}, false
	}
	descriptor, err := u.agents.Get(ctx, home, providerID)
	if err != nil {
		return engineagents.TerminalNotice{}, false
	}
	matcher, ok := descriptor.(engineagents.NoticeMatcher)
	if !ok {
		return engineagents.TerminalNotice{}, false
	}
	return matcher.MatchTerminalNotice(screen)
}

// OpenWork reports whether this chat's conversation record still shows something
// RUNNING: a tool call the provider opened and has not closed, or a subagent it
// started and has not stopped.
//
// This is the stall detector's one piece of evidence that does not come from the
// screen, and it is the gate that covers the provider whose painting behaviour
// while working has never been measured. A CLI mid-tool announced that tool over
// a hook and has not announced its end; that is proof it is working, whatever its
// screen looks like.
//
// It reads the PROJECTION rather than the aggregate because the aggregate's open
// maps are not exposed on the repository's interface, and it reads the whole of a
// chat's tool rows to do it — which is why the caller asks it LAST, only once
// every cheap gate has already agreed. On a healthy daemon it is never reached at
// all.
func (u *Usecase) OpenWork(ctx context.Context, chatID string) (bool, error) {
	tools, err := u.activity.ToolCalls(ctx, chatID, 0, 0)
	if err != nil {
		return false, err
	}
	for _, t := range tools {
		if t.Status == domain.ToolStatusRunning {
			return true, nil
		}
	}
	subagents, err := u.activity.Subagents(ctx, chatID)
	if err != nil {
		return false, err
	}
	for _, s := range subagents {
		if s.EndedAt == nil {
			return true, nil
		}
	}
	return false, nil
}
