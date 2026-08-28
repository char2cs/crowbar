package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type AnswerChoice struct {
	ChatID    string
	ChoiceID  string
	OptionIDs []string
	// Auto is true when policy resolved this prompt, false when a human clicked
	// it — the fact that lets the transcript tell the two apart.
	Auto bool
	Now  time.Time
}

func (c AnswerChoice) AggregateID() string  { return c.ChatID }
func (c AnswerChoice) EventName() string    { return "agentactivity.choice_answered." + c.ChatID }
func (c AnswerChoice) ShouldSnapshot() bool { return false }

func (c AnswerChoice) Validate(current *domain.ChatActivity) error {
	if err := requireChat("answer choice", c.ChatID); err != nil {
		return err
	}
	if err := requireID("answer choice", "choice id", c.ChoiceID); err != nil {
		return err
	}
	if len(c.OptionIDs) == 0 {
		return fmt.Errorf("answer choice: no option chosen: %w", asynxModels.ErrValidation)
	}
	if current == nil {
		return fmt.Errorf("answer choice: no such prompt: %w", asynxModels.ErrValidation)
	}
	choice, open := current.Choices[c.ChoiceID]
	if !open {
		return fmt.Errorf("answer choice: prompt is no longer pending: %w", asynxModels.ErrValidation)
	}
	return validateOptions(choice, c.OptionIDs)
}

func validateOptions(choice domain.ActivityChoice, picked []string) error {
	if _, err := choice.ResolvePicks(picked); err != nil {
		return fmt.Errorf("answer choice: %w: %w", err, asynxModels.ErrValidation)
	}
	return nil
}

func (c AnswerChoice) EmitEvent(current *domain.ChatActivity) domain.ChatActivity {
	next := advance(current, c.ChatID)
	item, known := next.Choices[c.ChoiceID]
	if !known {
		return next
	}
	delete(next.Choices, c.ChoiceID)
	if len(next.Choices) == 0 {
		next.Choices = nil
	}
	item.ResolvedAt = at(c.Now)
	item.Resolution = domain.ChoiceResolutionAnswered
	item.AutoApproved = c.Auto
	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaChoice, Choice: &item,
	}
	return next
}
