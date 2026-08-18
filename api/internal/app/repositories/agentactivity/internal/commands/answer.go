package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// AnswerChoice records a human deciding a prompt THROUGH Crowbar.
//
// It is the explicit counterpart of the resolutions the observation half already
// writes: ChoiceResolutionProceeded is the work being seen to move on because
// somebody answered at the PTY, and ChoiceResolutionAbandoned is the turn ending
// with the question still hanging. This is the third case, and the only one that
// is a DECISION rather than an inference — which is exactly why it is a command
// of its own rather than a ResolveChoice with a different string.
//
// What it adds over that is the check: an answer names the OPTION IDS it picked,
// and an option the prompt never offered is refused. Nothing downstream could
// catch that — the relay would render a decision key the prompt had no business
// producing, and the CLI would be told something the agent never asked about.
//
// The RECORD is unchanged by this. The picked ids are validated and then dropped:
// they are the payload of a decision, not a property of the question, and the one
// place they have to arrive is the blocked hook's stdout. Persisting them here
// would put a second, divergent copy of the answer in the log.
type AnswerChoice struct {
	ChatID    string
	ChoiceID  string
	OptionIDs []string
	Now       time.Time
}

func (c AnswerChoice) AggregateID() string  { return c.ChatID }
func (c AnswerChoice) EventName() string    { return "agentactivity.choice_answered." + c.ChatID }
func (c AnswerChoice) ShouldSnapshot() bool { return false }

func (c AnswerChoice) Validate(current *domain.AgentActivity) error {
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
		// A prompt that is not open is one somebody already answered — at the PTY, or
		// in another window — and an answer to it is stale rather than malformed. It
		// is refused here so the caller can say so, instead of appending a resolution
		// over a question that has one.
		return fmt.Errorf("answer choice: prompt is no longer pending: %w", asynxModels.ErrValidation)
	}
	return validateOptions(choice, c.OptionIDs)
}

// validateOptions checks every picked id against what the prompt actually asked.
//
// The rule itself is domain.ActivityChoice.ResolvePicks and is NOT restated here.
// It used to be: this command enforced "one pick unless Multi, and every id must
// be offered", while the usecase that renders the answer enforced "every pick of
// one kind" — two validators, two strictnesses, and neither of them noticed a
// PARTIAL answer to a multi-question prompt. One rule in one place is what stops
// the two from disagreeing again; all this does is restate its failure in the
// aggregate's own error vocabulary.
//
// A prompt with nothing to pick from is not unanswerable — an MCP elicitation
// offers a schema and expects a form back, and what it is told is accept, decline
// or cancel rather than a pick from a list. ResolvePicks says so by returning
// nothing, and the descriptor's response templates are what decide whether the
// verb means anything to the provider.
func validateOptions(choice domain.ActivityChoice, picked []string) error {
	if _, err := choice.ResolvePicks(picked); err != nil {
		return fmt.Errorf("answer choice: %w: %w", err, asynxModels.ErrValidation)
	}
	return nil
}

func (c AnswerChoice) EmitEvent(current *domain.AgentActivity) domain.AgentActivity {
	next := advance(current, c.ChatID)
	item, known := next.Choices[c.ChoiceID]
	if !known {
		// Validate already refused this, so reaching here means the aggregate moved
		// between the two. Emitting an event with NO delta is the same answer
		// ResolveChoice gives: the attempt is worth having in the log, and a
		// fabricated record would be projected over a real row and blank the question
		// it was asking.
		return next
	}
	delete(next.Choices, c.ChoiceID)
	if len(next.Choices) == 0 {
		next.Choices = nil
	}
	item.ResolvedAt = at(c.Now)
	item.Resolution = domain.ChoiceResolutionAnswered
	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaChoice, Choice: &item,
	}
	return next
}
