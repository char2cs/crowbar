package chat

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/ingress"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// AnswerUsecase is the human half of a provider prompt: the relay blocked on a
// person, and deciding for it. Its state is a desk of live relays; the verdict
// is recorded in the chat's ledger.
type AnswerUsecase interface {
	// PendingAnswer reports the prompt the relay held on askID is parked on, and
	// how long it may wait for a person before it must print nothing and let the
	// provider fall back to its own UI.
	PendingAnswer(
		askID string,
	) (PendingAnswer, bool)

	// AwaitAnswer blocks the relay held on askID until a person answers, its
	// declared budget expires, or ctx is cancelled. It returns the stdout the
	// relay must print — empty when nobody answered in time, which is the signal
	// to leave the prompt to the provider's own UI.
	//
	// A verdict is claimed by EXACTLY ONE relay: one that arrives after the
	// answer was already claimed gets nothing rather than replaying it.
	AwaitAnswer(
		ctx context.Context,
		askID string,
	) (HookAnswer, error)

	// AbandonAnswer retires the relay held on askID without printing. When no
	// verdict had been reached it also closes the ledger's question as proceeded,
	// because the provider is about to resolve the prompt through its own UI.
	AbandonAnswer(
		ctx context.Context,
		askID string,
	)

	// AnswerChoice decides a pending prompt from Crowbar: it renders the picked
	// options into the provider's own answer format and hands that to the relay
	// holding the prompt open.
	//
	// Returns ErrNoLongerWaiting (an apperr.ErrConflict) when no relay is still
	// holding the prompt — its CLI has moved on, so it can no longer be answered
	// from Crowbar — and apperr.ErrInvalidArgument when the provider cannot
	// express the decision.
	AnswerChoice(
		ctx context.Context,
		chatID string,
		choiceID string,
		optionIDs []string,
		reason string,
		content []byte,
	) error

	// AnswerableChoiceIDs filters choices down to the ones a relay of chatID is
	// still blocked on, which is what makes a pending question actionable in the
	// UI rather than merely visible.
	AnswerableChoiceIDs(
		chatID string,
		choices []domain.ActivityChoice,
	) []string

	// SetChatPermissionLevel overrides chatID's trust dial for the rest of its
	// lifetime, independent of the global default (see
	// ProviderUsecase.DefaultPermissionLevel). Refuses a level the chat's
	// current provider does not declare, with apperr.ErrInvalidArgument.
	SetChatPermissionLevel(
		ctx context.Context,
		chatID string,
		level string,
	) error
}

var _ AnswerUsecase = (*Usecase)(nil)

// ErrNoLongerWaiting refuses an answer to a prompt whose relay stopped waiting
// first, so the answer would reach no CLI.
var ErrNoLongerWaiting = answerdesk.ErrNotWaiting

// PendingAnswer is what a blocked hook relay is told: which prompt it is parked
// on, and how long it may wait for a person before it must print nothing and let
// the provider fall back to its own UI.
type PendingAnswer = answerdesk.PendingAnswer

// HookAnswer is what a relay prints. An empty Stdout means nobody answered in
// time.
type HookAnswer = answerdesk.HookAnswer

// The human in the loop: a provider prompt that blocks its hook relay until
// somebody decides, and the Crowbar-side act of deciding for it.
//
// The desk holding those relays is in memory and can never be otherwise: a slot
// describes a live hook process holding a live provider gate open, so one that
// survived a restart would be a promise to a process that no longer exists.

// PendingAnswer reports the prompt the relay held on askID is parked on, and how
// long it may wait for a person before it must print nothing.
func (u *Usecase) PendingAnswer(
	askID string,
) (PendingAnswer, bool) {
	return u.answers.Pending(askID)
}

// AwaitAnswer blocks the relay held on askID until a person answers, its declared
// budget expires, or ctx is cancelled. A verdict is claimed by exactly one relay,
// and a relay that hangs up or runs out prints nothing: its CLI asks it itself.
func (u *Usecase) AwaitAnswer(
	ctx context.Context,
	askID string,
) (HookAnswer, error) {
	answer, err := u.answers.Await(ctx, askID, domain.ChoiceResolutionProceeded)
	if err != nil && ctx.Err() != nil {
		u.answers.Abandon(context.WithoutCancel(ctx), askID)
	}
	return answer, err
}

// AbandonAnswer retires the relay held on askID without printing, closing the
// ledger's question as proceeded when nobody had decided it.
func (u *Usecase) AbandonAnswer(
	ctx context.Context,
	askID string,
) {
	u.answers.Abandon(ctx, askID)
}

// AnswerChoice decides a pending prompt from Crowbar: it renders the picked
// options into the provider's OWN answer format and hands that to the relay
// holding the prompt open.
//
// The order is load-bearing. Everything that can refuse — the chat, the relay, the
// question still being pending, the decision being expressible, the provider being
// resolvable, the render itself — is settled BEFORE the ledger is written, and the
// ledger is written only while the desk holds the relay for this answer alone, so
// a recorded answer is one its CLI is handed.
func (u *Usecase) AnswerChoice(
	ctx context.Context,
	chatID string,
	choiceID string,
	optionIDs []string,
	reason string,
	content []byte,
) error {
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		return fmt.Errorf("agent: answer choice: chat: %w", err)
	}
	slot, held := u.answers.ByChoiceID(choiceID)
	if !held || slot.ChatID != chatID {
		return ErrNoLongerWaiting
	}
	choice, found, err := u.pendingChoice(ctx, chatID, choiceID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: this prompt is no longer pending", apperr.ErrConflict)
	}

	decision, err := answerdesk.Decide(choice, optionIDs, reason, content)
	if err != nil {
		return err
	}
	if !slot.Keys.Accepts(decision.Key) {
		return fmt.Errorf("%w: this provider cannot express that answer", apperr.ErrInvalidArgument)
	}
	agent, err := u.agentForChat(ctx, chatID)
	if err != nil {
		return err
	}

	stdout, err := agent.RenderAnswer(slot.Event, slot.Raw, decision)
	if err != nil {
		return fmt.Errorf("agent: answer choice: render: %w", err)
	}

	answered, err := u.answers.Answer(ctx, slot, stdout, func(ctx context.Context) (bool, error) {
		return u.asks.resolve(ctx, chatID, choiceID, ledger.ResolutionAnswered, optionIDs)
	})
	switch {
	case errors.Is(err, ErrNoLongerWaiting):
		return err
	case err != nil:
		return fmt.Errorf("agent: answer choice: %w", err)
	case !answered:
		return fmt.Errorf("%w: this prompt is no longer pending", apperr.ErrConflict)
	}
	return nil
}

// askResolver closes a chat's asks: the outcome a relay reached, or a person's
// answer.
type askResolver struct{ writes ingress.Writer }

func (r askResolver) ResolveAsk(ctx context.Context, chatID, askID, resolution string) error {
	if _, err := r.resolve(ctx, chatID, askID, resolution, nil); err != nil {
		return fmt.Errorf("agent: resolve ask: %w", err)
	}
	return nil
}

// resolve closes askID as resolution, and reports whether it was still open.
func (r askResolver) resolve(ctx context.Context, chatID, askID, resolution string, optionIDs []string) (bool, error) {
	rows, err := r.writes.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		return ledger.ResolveAsk(st, askID, resolution, optionIDs)
	})
	return len(rows) > 0, err
}

// pendingChoice finds the still-open question by id. A prompt the CLI has already
// resolved its own way is gone from here, which is what makes "no longer pending"
// a real refusal rather than a race.
func (u *Usecase) pendingChoice(
	ctx context.Context,
	chatID string,
	choiceID string,
) (domain.ActivityChoice, bool, error) {
	choices, err := u.transcript.PendingChoices(ctx, chatID)
	if err != nil {
		return domain.ActivityChoice{}, false, fmt.Errorf("agent: answer choice: pending: %w", err)
	}
	for _, choice := range choices {
		if choice.ID == choiceID {
			return choice, true, nil
		}
	}
	return domain.ActivityChoice{}, false, nil
}

// agentForChat resolves the descriptor that will render the answer. It goes
// through the LIVE runner rather than the chat's stored provider: the answer has
// to be in the format of the CLI that is actually holding the prompt open.
func (u *Usecase) agentForChat(
	ctx context.Context,
	chatID string,
) (engineagents.Agent, error) {
	runner, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("agent: answer choice: runner: %w", err)
	}
	crowbarHome, _, _, _, err := u.ws.WorktreeDir(ctx, runner.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("agent: answer choice: worktree dir: %w", err)
	}
	agent, err := u.agents.Get(ctx, crowbarHome, runner.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("agent: answer choice: descriptor: %w", err)
	}
	return agent, nil
}

// AnswerableChoiceIDs filters choices down to the ones a relay of chatID is still
// blocked on — what makes a pending question actionable rather than merely visible.
func (u *Usecase) AnswerableChoiceIDs(
	chatID string,
	choices []domain.ActivityChoice,
) []string {
	return u.answers.AnswerableIDs(chatID, choices)
}
