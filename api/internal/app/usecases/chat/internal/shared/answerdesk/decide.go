package answerdesk

import (
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Decide turns the options a person picked into the decision a provider can be
// asked to render.
//
// The picked options must be of ONE kind: a decision that both allows and denies
// is not a decision, and no provider's answer format can express it.
func Decide(
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
