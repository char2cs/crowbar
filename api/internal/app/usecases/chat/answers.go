package chat

import (
	"fmt"
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

func decide(
	choice domain.ActivityChoice,
	optionIDs []string,
	reason string,
	content []byte,
) (engineagents.AnswerDecision, error) {
	if len(optionIDs) == 0 {
		return engineagents.AnswerDecision{}, fmt.Errorf(
			"%w: an answer must name at least one option", apperr.ErrInvalidArgument,
		)
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
					"%w: an answer must pick options of one kind", apperr.ErrInvalidArgument,
				)
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
