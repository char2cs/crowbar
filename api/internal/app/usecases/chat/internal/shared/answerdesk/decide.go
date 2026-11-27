package answerdesk

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

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
	if key == domain.ChoiceOptionDeny {
		said := strings.TrimSpace(decision.Reason)
		decision.Reason = strings.TrimSuffix("The user denied this request and said: "+said, " and said: ")
	}
	if key == domain.ChoiceOptionSuggestion {
		decision.Picked = optionIDs
	}
	if key == domain.ChoiceOptionAnswer {
		decision.Answers = answersByQuestion(answers)
	}
	return decision, nil
}

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
