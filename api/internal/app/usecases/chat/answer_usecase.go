package chat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// AnswerUsecase is the human-in-the-loop half of a provider prompt: the hook
// relay that is BLOCKED waiting for a person, and the Crowbar-side act of
// deciding for it.
//
// Its whole state is a desk of live relays (see answerDesk), which is why it
// holds no durable store of its own beyond the activity ledger it records the
// verdict in.
type AnswerUsecase interface {
	// PendingAnswer reports the prompt a hook delivery is parked on, and how long
	// the relay may wait for a person before it must print nothing and let the
	// provider fall back to its own UI.
	PendingAnswer(
		deliveryID string,
	) (PendingAnswer, bool)

	// AwaitAnswer blocks the relay of deliveryID until a person answers, its
	// declared budget expires, or ctx is cancelled. It returns the stdout the
	// relay must print — empty when nobody answered in time, which is the signal
	// to leave the prompt to the provider's own UI.
	//
	// A verdict is claimed by EXACTLY ONE relay: a retried delivery that arrives
	// after the answer was already claimed gets nothing rather than replaying it.
	AwaitAnswer(
		ctx context.Context,
		deliveryID string,
	) (HookAnswer, error)

	// AbandonAnswer retires the relay of deliveryID without printing. When no
	// verdict had been reached it also closes the ledger's question as proceeded,
	// because the provider is about to resolve the prompt through its own UI.
	AbandonAnswer(
		ctx context.Context,
		deliveryID string,
	) error

	// AnswerChoice decides a pending prompt from Crowbar: it renders the picked
	// options into the provider's own answer format and hands that to the relay
	// holding the prompt open.
	//
	// Returns apperr.ErrConflict when no relay is still holding the prompt — a
	// prompt whose CLI has moved on can no longer be answered from Crowbar — and
	// apperr.ErrInvalidArgument when the provider cannot express the decision.
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
}

var _ AnswerUsecase = (*answerUsecase)(nil)

type answerUsecase struct {
	activity agentactivity.EventStore
	chats    agentchat.EventStore
	runners  agentrunner.EventStore
	agents   engineagents.Agents
	ws       WorkspaceReader
	// answers is the desk of relays currently BLOCKED on a human. It is in memory
	// because a slot describes a live hook process holding a live provider gate
	// open; see answers.go.
	answers *answerDesk
}

func newAnswerUsecase(
	activity agentactivity.EventStore,
	chats agentchat.EventStore,
	runners agentrunner.EventStore,
	agents engineagents.Agents,
	ws WorkspaceReader,
) *answerUsecase {
	return &answerUsecase{
		activity: activity,
		chats:    chats,
		runners:  runners,
		agents:   agents,
		ws:       ws,
		answers:  newAnswerDesk(),
	}
}

func (u *answerUsecase) holdForAnswer(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
	choiceID string,
	raw []byte,
) {
	deliveryID := hookDeliveryID(ctx)
	if deliveryID == "" || choiceID == "" {
		return
	}
	capability, answerable := agent.AnswerCapability(ev.Kind)
	if !answerable {
		return
	}
	if len(raw) > maxAnswerPayloadBytes {
		slog.DebugContext(ctx, "agent: answer: prompt payload too large to answer",
			"chat_id", chat.ID, "event", ev.Kind, "bytes", len(raw))
		return
	}
	slot := u.answers.open(deliveryID, &answerSlot{
		choiceID: choiceID,
		chatID:   chat.ID,
		runnerID: runner.ID,
		event:    ev.Kind,

		raw:  append([]byte(nil), raw...),
		keys: capability,
		done: make(chan struct{}),
	})
	_ = slot
}

func (u *answerUsecase) PendingAnswer(
	deliveryID string,
) (PendingAnswer, bool) {
	slot, held := u.answers.byDeliveryID(deliveryID)
	if !held {
		return PendingAnswer{}, false
	}
	return PendingAnswer{ChoiceID: slot.choiceID, Wait: answerWait(slot.keys.Wait)}, true
}

func (u *answerUsecase) AwaitAnswer(
	ctx context.Context,
	deliveryID string,
) (HookAnswer, error) {
	slot, held := u.answers.byDeliveryID(deliveryID)
	if !held {
		return HookAnswer{}, nil
	}
	if stdout, claimed := u.answers.claim(slot); claimed {
		return HookAnswer{Stdout: stdout}, nil
	}
	timer := time.NewTimer(answerWait(slot.keys.Wait))
	defer timer.Stop()
	select {
	case <-slot.done:
		stdout, _ := u.answers.claim(slot)
		return HookAnswer{Stdout: stdout}, nil
	case <-timer.C:

		if stdout, claimed := u.answers.claim(slot); claimed {
			return HookAnswer{Stdout: stdout}, nil
		}
		u.answers.release(slot)
		return HookAnswer{}, nil
	case <-ctx.Done():

		u.answers.release(slot)
		return HookAnswer{}, ctx.Err()
	}
}

func (u *answerUsecase) AbandonAnswer(
	ctx context.Context,
	deliveryID string,
) error {
	slot, held := u.answers.byDeliveryID(deliveryID)
	if !held {
		return nil
	}
	if decided := u.answers.discard(slot); decided {
		return nil
	}
	note(ctx, "choice resolved elsewhere", u.activity.ResolveChoice(
		ctx, slot.chatID, slot.choiceID, domain.ChoiceResolutionProceeded, time.Now(),
	))
	return nil
}

func (u *answerUsecase) AnswerChoice(
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
	slot, held := u.answers.byChoiceID(choiceID)
	if !held || slot.chatID != chatID {
		return fmt.Errorf("%w: this prompt can no longer be answered from Crowbar",
			apperr.ErrConflict)
	}
	choice, found, err := u.pendingChoice(ctx, chatID, choiceID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: this prompt is no longer pending", apperr.ErrConflict)
	}

	decision, err := decide(choice, optionIDs, reason, content)
	if err != nil {
		return err
	}
	if !slot.keys.Accepts(decision.Key) {
		return fmt.Errorf("%w: this provider cannot express that answer", apperr.ErrInvalidArgument)
	}
	agent, err := u.agentForChat(ctx, chatID)
	if err != nil {
		return err
	}

	stdout, err := agent.RenderAnswer(slot.event, slot.raw, decision)
	if err != nil {
		return fmt.Errorf("agent: answer choice: render: %w", err)
	}

	if err := u.activity.AnswerChoice(ctx, chatID, choiceID, optionIDs, time.Now()); err != nil {
		return fmt.Errorf("agent: answer choice: %w", err)
	}
	u.answers.resolve(slot, stdout)
	return nil
}

func (u *answerUsecase) pendingChoice(
	ctx context.Context,
	chatID string,
	choiceID string,
) (domain.ActivityChoice, bool, error) {
	choices, err := u.activity.PendingChoices(ctx, chatID)
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

func (u *answerUsecase) agentForChat(
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

func (u *answerUsecase) AnswerableChoiceIDs(
	chatID string,
	choices []domain.ActivityChoice,
) []string {
	out := make([]string, 0, len(choices))
	for _, choice := range choices {
		if slot, held := u.answers.byChoiceID(choice.ID); held && slot.chatID == chatID {
			out = append(out, choice.ID)
		}
	}
	return out
}

func (u *answerUsecase) releaseAnswerWaiters(
	ctx context.Context,
	runnerID string,
) {
	for _, slot := range u.answers.releaseRunner(runnerID) {
		err := u.activity.ResolveChoice(
			ctx, slot.chatID, slot.choiceID, domain.ChoiceResolutionAbandoned, time.Now(),
		)
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.WarnContext(ctx, "agent: answer: release prompt of dead runner",
				"runner_id", runnerID, "choice_id", slot.choiceID, "err", err)
		}
	}
}
