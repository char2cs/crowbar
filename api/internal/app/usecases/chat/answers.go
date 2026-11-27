package chat

import (
	"context"
	"errors"
	"fmt"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/ingress"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type AnswerUsecase interface {
	HoldsAnswer(
		askID string,
	) bool

	AwaitAnswer(
		ctx context.Context,
		askID string,
	) (HookAnswer, error)

	AnswerChoice(
		ctx context.Context,
		chatID string,
		choiceID string,
		optionIDs []string,
		reason string,
		content []byte,
	) error

	AnswerTerminalWait(
		ctx context.Context,
		chatID string,
		answerID string,
	) error

	SetChatPermissionLevel(
		ctx context.Context,
		chatID string,
		level string,
	) error
}

var _ AnswerUsecase = (*Usecase)(nil)

var ErrNoLongerWaiting = answerdesk.ErrNotWaiting

type HookAnswer = answerdesk.HookAnswer

func (u *Usecase) HoldsAnswer(
	askID string,
) bool {
	return u.answers.Holds(askID)
}

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
		return fmt.Errorf("%w: this prompt is no longer pending", problem.ErrConflict)
	}

	decision, err := answerdesk.Decide(choice, optionIDs, reason, content)
	if err != nil {
		return err
	}
	if !slot.Keys.Accepts(decision.Key) {
		return fmt.Errorf("%w: this provider cannot express that answer", problem.ErrInvalidArgument)
	}
	agent, err := u.agentForChat(ctx, chatID)
	if err != nil {
		return err
	}

	stdout, err := agent.RenderAnswer(slot.Event, slot.Raw, decision)
	if err != nil {
		return fmt.Errorf("agent: answer choice: render: %w", err)
	}

	answered, err := u.answers.Answer(ctx, slot, stdout, func(ctx context.Context, waiting func() bool) (bool, error) {
		return u.asks.resolve(ctx, chatID, choiceID, ledger.ResolutionAnswered, optionIDs, waiting)
	})
	switch {
	case errors.Is(err, ErrNoLongerWaiting):
		return err
	case err != nil:
		return fmt.Errorf("agent: answer choice: %w", err)
	case !answered:
		return fmt.Errorf("%w: this prompt is no longer pending", problem.ErrConflict)
	}
	return nil
}

type askResolver struct{ writes ingress.Writer }

func (r askResolver) ResolveAsk(ctx context.Context, chatID, askID, resolution string) error {
	if _, err := r.resolve(ctx, chatID, askID, resolution, nil, always); err != nil {
		return fmt.Errorf("agent: resolve ask: %w", err)
	}
	return nil
}

func always() bool { return true }

func (r askResolver) resolve(
	ctx context.Context, chatID, askID, resolution string, optionIDs []string, may func() bool,
) (bool, error) {
	rows, err := r.writes.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		if !may() {
			return nil, nil
		}
		return ledger.ResolveAsk(st, askID, resolution, optionIDs)
	})
	return len(rows) > 0, err
}

type openAsks struct{ ledger *ledgerstore.Store }

func (o openAsks) PendingChoices(ctx context.Context, chatID string) ([]domain.ActivityChoice, error) {
	st, err := o.ledger.State(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("agent: open asks: %w", err)
	}
	return st.OpenAsks(), nil
}

func (u *Usecase) pendingChoice(
	ctx context.Context,
	chatID string,
	choiceID string,
) (domain.ActivityChoice, bool, error) {
	choices, err := openAsks{ledger: u.ledger}.PendingChoices(ctx, chatID)
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

func (u *Usecase) AnswerTerminalWait(ctx context.Context, chatID, answerID string) error {
	return u.runners.AnswerTerminalWait(ctx, chatID, answerID)
}
