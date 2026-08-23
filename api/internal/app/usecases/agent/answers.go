package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

const (
	maxAnswerPayloadBytes = 128 << 10

	answerWaitFallback = 120 * time.Second

	maxAnswerWait = 15 * time.Minute

	answerVerdictRetention = time.Minute
)

type PendingAnswer struct {
	ChoiceID string

	Wait time.Duration
}

type HookAnswer struct {
	Stdout []byte
}

type answerSlot struct {
	choiceID string
	chatID   string
	runnerID string

	event string
	raw   []byte

	keys engineagents.AnswerCapability

	done   chan struct{}
	stdout []byte
	once   sync.Once

	decidedAt time.Time
	spent     bool
	reap      *time.Timer
}

func (s *answerSlot) settle(stdout []byte) {
	s.once.Do(func() {
		s.stdout = stdout
		close(s.done)
	})
}

type answerDesk struct {
	mu         sync.Mutex
	byChoice   map[string]*answerSlot
	byDelivery map[string]*answerSlot

	retention time.Duration
}

func newAnswerDesk() *answerDesk {
	return &answerDesk{
		byChoice:   map[string]*answerSlot{},
		byDelivery: map[string]*answerSlot{},
		retention:  answerVerdictRetention,
	}
}

func (d *answerDesk) open(deliveryID string, slot *answerSlot) *answerSlot {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dropExpiredLocked()
	if existing, held := d.byDelivery[deliveryID]; held {
		return existing
	}

	if stale, held := d.byChoice[slot.choiceID]; held {
		d.forgetLocked(stale)
		stale.settle(nil)
	}
	d.byDelivery[deliveryID] = slot
	d.byChoice[slot.choiceID] = slot
	return slot
}

func (d *answerDesk) byDeliveryID(deliveryID string) (*answerSlot, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dropExpiredLocked()
	slot, ok := d.byDelivery[deliveryID]
	return slot, ok
}

func (d *answerDesk) byChoiceID(choiceID string) (*answerSlot, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	slot, ok := d.byChoice[choiceID]
	return slot, ok
}

func (d *answerDesk) resolve(slot *answerSlot, stdout []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if slot.spent {
		return
	}
	if held, ok := d.byChoice[slot.choiceID]; ok && held == slot {
		delete(d.byChoice, slot.choiceID)
	}
	slot.decidedAt = time.Now()
	slot.reap = time.AfterFunc(d.retention, d.dropExpired)
	slot.settle(stdout)
}

func (d *answerDesk) claim(slot *answerSlot) ([]byte, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.claimableLocked(slot) {
		return nil, false
	}
	d.forgetLocked(slot)
	return slot.stdout, true
}

func (d *answerDesk) release(slot *answerSlot) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.claimableLocked(slot) {
		return
	}
	d.forgetLocked(slot)
	slot.settle(nil)
}

func (d *answerDesk) discard(slot *answerSlot) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	decided := d.claimableLocked(slot)
	d.forgetLocked(slot)
	slot.settle(nil)
	return decided
}

func (d *answerDesk) dropExpired() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dropExpiredLocked()
}

func (d *answerDesk) claimableLocked(slot *answerSlot) bool {
	return !slot.spent &&
		!slot.decidedAt.IsZero() &&
		time.Since(slot.decidedAt) < d.retention
}

func (d *answerDesk) forgetLocked(slot *answerSlot) {
	for id, held := range d.byDelivery {
		if held == slot {
			delete(d.byDelivery, id)
		}
	}
	if held, ok := d.byChoice[slot.choiceID]; ok && held == slot {
		delete(d.byChoice, slot.choiceID)
	}
	slot.spent = true
	if slot.reap != nil {
		slot.reap.Stop()
		slot.reap = nil
	}
}

func (d *answerDesk) dropExpiredLocked() {
	var stale []*answerSlot
	for _, slot := range d.byDelivery {
		if !slot.decidedAt.IsZero() && time.Since(slot.decidedAt) >= d.retention {
			stale = append(stale, slot)
		}
	}
	for _, slot := range stale {
		d.forgetLocked(slot)
	}
}

func (d *answerDesk) releaseRunner(runnerID string) []*answerSlot {
	d.mu.Lock()
	var blocked []*answerSlot
	for _, slot := range d.slotsOfLocked(runnerID) {
		decided := !slot.decidedAt.IsZero()
		d.forgetLocked(slot)
		if decided {
			continue
		}
		slot.settle(nil)
		blocked = append(blocked, slot)
	}
	d.mu.Unlock()
	return blocked
}

func (d *answerDesk) slotsOfLocked(runnerID string) []*answerSlot {
	owned := map[*answerSlot]bool{}
	for _, slot := range d.byChoice {
		owned[slot] = slot.runnerID == runnerID
	}
	for _, slot := range d.byDelivery {
		owned[slot] = slot.runnerID == runnerID
	}
	slots := make([]*answerSlot, 0, len(owned))
	for slot, mine := range owned {
		if mine {
			slots = append(slots, slot)
		}
	}
	return slots
}

func (u *Usecase) holdForAnswer(
	ctx context.Context,
	chat domain.AgentChat,
	runner domain.AgentRunner,
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

func (u *Usecase) PendingAnswer(deliveryID string) (PendingAnswer, bool) {
	slot, held := u.answers.byDeliveryID(deliveryID)
	if !held {
		return PendingAnswer{}, false
	}
	return PendingAnswer{ChoiceID: slot.choiceID, Wait: answerWait(slot.keys.Wait)}, true
}

func answerWait(declared time.Duration) time.Duration {
	switch {
	case declared <= 0:
		return answerWaitFallback
	case declared > maxAnswerWait:
		return maxAnswerWait
	default:
		return declared
	}
}

func (u *Usecase) AwaitAnswer(ctx context.Context, deliveryID string) (HookAnswer, error) {
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

func (u *Usecase) AbandonAnswer(ctx context.Context, deliveryID string) error {
	slot, held := u.answers.byDeliveryID(deliveryID)
	if !held {
		return nil
	}
	if decided := u.answers.discard(slot); decided {
		return nil
	}
	u.note(ctx, "choice resolved elsewhere", u.activity.ResolveChoice(
		ctx, slot.chatID, slot.choiceID, domain.ChoiceResolutionProceeded, time.Now(),
	))
	return nil
}

func (u *Usecase) AnswerChoice(
	ctx context.Context,
	chatID, choiceID string,
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

func decide(
	choice domain.ActivityChoice,
	optionIDs []string,
	reason string,
	content []byte,
) (engineagents.AnswerDecision, error) {
	if len(optionIDs) == 0 {
		return engineagents.AnswerDecision{}, fmt.Errorf(
			"%w: an answer must name at least one option", apperr.ErrInvalidArgument)
	}
	decision := engineagents.AnswerDecision{Reason: reason, Content: content}
	answers, err := choice.ResolvePicks(optionIDs)
	if err != nil {
		return engineagents.AnswerDecision{}, fmt.Errorf("%w: %w", apperr.ErrInvalidArgument, err)
	}
	if len(answers) == 0 {

		decision.Key = optionIDs[0]
		return decision, nil
	}

	key, err := decisionKey(answers)
	if err != nil {
		return engineagents.AnswerDecision{}, err
	}
	decision.Key = key
	if key == domain.ChoiceOptionAnswer {
		decision.Answers = answersByQuestion(answers)
	}
	return decision, nil
}

func decisionKey(answers []domain.ChoiceAnswer) (string, error) {
	key := ""
	for _, answer := range answers {
		for _, option := range answer.Picked {
			if key != "" && key != option.Kind {
				return "", fmt.Errorf(
					"%w: an answer must pick options of one kind", apperr.ErrInvalidArgument)
			}
			key = option.Kind
		}
	}
	return key, nil
}

func answersByQuestion(answers []domain.ChoiceAnswer) map[string]any {
	out := make(map[string]any, len(answers))
	for _, answer := range answers {
		key := answer.Question.AnswerKey()
		if key == "" {

			continue
		}
		labels := make([]any, 0, len(answer.Picked))
		for _, option := range answer.Picked {
			labels = append(labels, option.Label)
		}

		if answer.Question.Multi || len(labels) != 1 {
			out[key] = labels
			continue
		}
		out[key] = labels[0]
	}
	return out
}

func (u *Usecase) pendingChoice(
	ctx context.Context,
	chatID, choiceID string,
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

func (u *Usecase) agentForChat(ctx context.Context, chatID string) (engineagents.Agent, error) {
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

func (u *Usecase) AnswerableChoiceIDs(chatID string, choices []domain.ActivityChoice) []string {
	out := make([]string, 0, len(choices))
	for _, choice := range choices {
		if slot, held := u.answers.byChoiceID(choice.ID); held && slot.chatID == chatID {
			out = append(out, choice.ID)
		}
	}
	return out
}

func (u *Usecase) releaseAnswerWaiters(ctx context.Context, runnerID string) {
	for _, slot := range u.answers.releaseRunner(runnerID) {
		err := u.activity.ResolveChoice(
			ctx, slot.chatID, slot.choiceID, domain.ChoiceResolutionAbandoned, time.Now())
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.WarnContext(ctx, "agent: answer: release prompt of dead runner",
				"runner_id", runnerID, "choice_id", slot.choiceID, "err", err)
		}
	}
}
