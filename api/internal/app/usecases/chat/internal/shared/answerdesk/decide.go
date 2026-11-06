package answerdesk

import (
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Decide turns the options a person picked into a decision a provider can render. The options must be
// of one kind: no provider's answer format can express a decision that both allows and denies.
func Decide(
	choice domain.ActivityChoice,
	optionIDs []string,
	reason string,
	content []byte,
) (engineagents.AnswerDecision, error) {
	if len(optionIDs) == 0 {
		return engineagents.AnswerDecision{}, fmt.Errorf(
			"%w: an answer must name at least one option", problem.ErrInvalidArgument,
		)
	}
	decision := engineagents.AnswerDecision{Reason: reason, Content: content}
	answers, err := choice.ResolvePicks(optionIDs)
	if err != nil {
		return engineagents.AnswerDecision{}, fmt.Errorf("%w: %w", problem.ErrInvalidArgument, err)
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
	// A provider reads a deny with no message as its own hook's refusal.
	if key == domain.ChoiceOptionDeny && decision.Reason == "" {
		decision.Reason = personDenied
	}
	if key == domain.ChoiceOptionAnswer {
		decision.Answers = answersByQuestion(answers)
	}
	return decision, nil
}

// personDenied is what a deny says when the person gave no reason of their own.
const personDenied = "The user denied this request."

// RefusalKey is the canonical reply that declines an ask of kind event without
// ending the turn — what a desk sends when nobody answered in time.
func RefusalKey(event string) string {
	if event == engineagents.HookElicitation {
		return "decline"
	}
	return "deny"
}

func decisionKey(answers []domain.ChoiceAnswer) (string, error) {
	key := ""
	for _, answer := range answers {
		for _, option := range answer.Picked {
			if key != "" && key != option.Kind {
				return "", fmt.Errorf(
					"%w: an answer must pick options of one kind", problem.ErrInvalidArgument,
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
