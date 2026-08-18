package commands_test

import (
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// pendingPermission is a chat mid-reply with one open allow/deny prompt — the
// state every answer is taken against.
func pendingPermission(t *testing.T) domain.AgentActivity {
	t.Helper()
	state := openTurnState(t)
	return permissionCmd().EmitEvent(&state)
}

func answerCmd(optionIDs ...string) commands.AnswerChoice {
	return commands.AnswerChoice{ChatID: chat, ChoiceID: "c1", OptionIDs: optionIDs, Now: now}
}

func TestAnswerChoice_ResolvesTheOpenPromptAsAnswered(t *testing.T) {
	state := pendingPermission(t)

	require.NoError(t, answerCmd("allow").Validate(&state))
	got := answerCmd("allow").EmitEvent(&state)

	require.NotNil(t, got.Last)
	assert.Equal(t, domain.DeltaClose, got.Last.Phase)
	assert.Equal(t, domain.DeltaChoice, got.Last.Kind)
	require.NotNil(t, got.Last.Choice)
	assert.Equal(t, domain.ChoiceResolutionAnswered, got.Last.Choice.Resolution)
	require.NotNil(t, got.Last.Choice.ResolvedAt)
	assert.Empty(t, got.Choices, "an answered prompt is no longer open state")
	assert.False(t, got.Last.Choice.Pending())
}

// The whole reason this is a command rather than a ResolveChoice with a
// different string: nothing downstream could catch an option the prompt never
// offered, because by then the decision has already been rendered into a
// provider's own JSON and printed on a hook.
func TestAnswerChoice_RefusesAnOptionThePromptNeverOffered(t *testing.T) {
	state := pendingPermission(t)

	err := answerCmd("definitely-not-an-option").Validate(&state)

	require.ErrorIs(t, err, asynxModels.ErrValidation)
	assert.Contains(t, err.Error(), "not an option")
}

func TestAnswerChoice_RefusesMoreThanOnePickOnASingleAnswerPrompt(t *testing.T) {
	state := pendingPermission(t)

	err := answerCmd("allow", "deny").Validate(&state)

	require.ErrorIs(t, err, asynxModels.ErrValidation)
	assert.Contains(t, err.Error(), "takes one answer")
}

// The two validators used to be written twice and enforce different strictnesses:
// this command checked "one pick unless Multi, and every id must be offered",
// while the usecase that renders the answer checked "every pick of one kind" —
// and NEITHER of them noticed a partial answer to a multi-question prompt. Both
// now call the one rule on the domain type, so an answer the aggregate accepts is
// exactly an answer the usecase can render.
func TestRegression_TheAggregateRefusesAPartialAnswerToAMultiQuestionPrompt(t *testing.T) {
	state := openTurnState(t)
	open := permissionCmd()
	open.Kind = domain.ChoiceKindQuestion
	open.Options = nil
	open.Questions = []domain.ActivityChoiceQuestion{
		{ID: "q0", Text: "Which language?", Options: []domain.ActivityChoiceOption{
			{ID: "q0-answer-0", Kind: domain.ChoiceOptionAnswer, Label: "Go"},
		}},
		{ID: "q1", Text: "Which databases?", Multi: true, Options: []domain.ActivityChoiceOption{
			{ID: "q1-answer-0", Kind: domain.ChoiceOptionAnswer, Label: "SQLite"},
			{ID: "q1-answer-1", Kind: domain.ChoiceOptionAnswer, Label: "Redis"},
		}},
	}
	state = open.EmitEvent(&state)

	err := answerCmd("q0-answer-0").Validate(&state)
	require.ErrorIs(t, err, asynxModels.ErrValidation)
	assert.Contains(t, err.Error(), "every question must be answered")

	require.NoError(t, answerCmd("q0-answer-0", "q1-answer-0", "q1-answer-1").Validate(&state),
		"an answer covering every question is accepted, multi-select and all")
}

func TestAnswerChoice_AcceptsSeveralPicksOnAMultiSelectPrompt(t *testing.T) {
	state := openTurnState(t)
	open := permissionCmd()
	open.Multi = true
	open.Options = []domain.ActivityChoiceOption{
		{ID: "answer-0", Kind: domain.ChoiceOptionAnswer, Label: "A"},
		{ID: "answer-1", Kind: domain.ChoiceOptionAnswer, Label: "B"},
	}
	state = open.EmitEvent(&state)

	require.NoError(t, answerCmd("answer-0", "answer-1").Validate(&state))
}

// An elicitation offers a SCHEMA, not a list, so there is nothing to check the
// ids against — what it is told is accept, decline or cancel, and whether that
// verb means anything is the descriptor's business.
func TestAnswerChoice_APromptWithNoOptionsAcceptsAProviderVerb(t *testing.T) {
	state := openTurnState(t)
	state = commands.OpenChoice{
		ChatID: chat, ChoiceID: "c1", Kind: domain.ChoiceKindElicitation,
		Question: "do you prefer A or B?", Now: now,
	}.EmitEvent(&state)

	require.NoError(t, answerCmd("accept").Validate(&state))
}

// A prompt that is not open was already answered — at the PTY, or in another
// window. That is stale rather than malformed, and it is refused so the caller
// can say so instead of appending a second resolution over the first.
func TestAnswerChoice_RefusesAPromptThatIsNoLongerPending(t *testing.T) {
	state := pendingPermission(t)
	answered := answerCmd("allow").EmitEvent(&state)

	err := answerCmd("allow").Validate(&answered)

	require.ErrorIs(t, err, asynxModels.ErrValidation)
	assert.Contains(t, err.Error(), "no longer pending")
}

func TestAnswerChoice_RefusesAnEmptyDecision(t *testing.T) {
	state := pendingPermission(t)

	err := answerCmd().Validate(&state)

	require.ErrorIs(t, err, asynxModels.ErrValidation)
	assert.Contains(t, err.Error(), "no option chosen")
}

func TestAnswerChoice_RefusesIncompleteIdentity(t *testing.T) {
	state := pendingPermission(t)

	err := commands.AnswerChoice{ChoiceID: "c1", OptionIDs: []string{"allow"}, Now: now}.
		Validate(&state)
	require.ErrorIs(t, err, asynxModels.ErrValidation)

	err = commands.AnswerChoice{ChatID: chat, OptionIDs: []string{"allow"}, Now: now}.
		Validate(&state)
	require.ErrorIs(t, err, asynxModels.ErrValidation)

	err = answerCmd("allow").Validate(nil)
	require.ErrorIs(t, err, asynxModels.ErrValidation)
	assert.Contains(t, err.Error(), "no such prompt")
}

// Validate refused this, so reaching EmitEvent means the aggregate moved between
// the two. The event is still emitted — the attempt belongs in the log — but with
// NO delta, because a fabricated record would be projected over a real row and
// blank the question it was asking.
func TestAnswerChoice_AnswerOfAVanishedPromptEmitsNoDelta(t *testing.T) {
	state := openTurnState(t)

	got := answerCmd("allow").EmitEvent(&state)

	assert.Nil(t, got.Last)
	assert.Equal(t, state.Seq+1, got.Seq)
}

func TestAnswerChoice_Identity(t *testing.T) {
	cmd := answerCmd("allow")
	assert.Equal(t, chat, cmd.AggregateID())
	assert.Equal(t, "agentactivity.choice_answered."+chat, cmd.EventName())
	assert.False(t, cmd.ShouldSnapshot())
}
