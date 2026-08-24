package answerdesk_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func choices(ids ...string) []domain.ActivityChoice {
	out := make([]domain.ActivityChoice, 0, len(ids))
	for _, id := range ids {
		out = append(out, domain.ActivityChoice{ID: id})
	}
	return out
}

func permission() domain.ActivityChoice {
	return domain.ActivityChoice{
		ID: "choice-1",
		Options: []domain.ActivityChoiceOption{
			{ID: "opt-allow", Kind: domain.ChoiceOptionAllow, Label: "Allow"},
			{ID: "opt-deny", Kind: domain.ChoiceOptionDeny, Label: "Deny"},
		},
	}
}

func TestDecide_RefusesAnAnswerThatNamesNoOption(t *testing.T) {
	t.Parallel()

	_, err := answerdesk.Decide(permission(), nil, "", nil)

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

func TestDecide_RefusesAnOptionTheChoiceDoesNotOffer(t *testing.T) {
	t.Parallel()

	_, err := answerdesk.Decide(permission(), []string{"opt-nonsense"}, "", nil)

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

// Allow-and-deny is not a decision, and no provider's answer format can carry it.
// The choice is deliberately multi-pick, so the domain's own arity check lets the
// pair through and the kind check is the thing under test.
func TestDecide_RefusesOptionsOfTwoKinds(t *testing.T) {
	t.Parallel()

	choice := domain.ActivityChoice{
		ID: "choice-1", Multi: true,
		Options: []domain.ActivityChoiceOption{
			{ID: "opt-allow", Kind: domain.ChoiceOptionAllow, Label: "Allow"},
			{ID: "opt-deny", Kind: domain.ChoiceOptionDeny, Label: "Deny"},
		},
	}

	_, err := answerdesk.Decide(choice, []string{"opt-allow", "opt-deny"}, "", nil)

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
	assert.Contains(t, err.Error(), "one kind")
}

func TestDecide_CarriesTheKindReasonAndContent(t *testing.T) {
	t.Parallel()

	decision, err := answerdesk.Decide(permission(), []string{"opt-allow"}, "because", []byte("body"))

	require.NoError(t, err)
	assert.Equal(t, domain.ChoiceOptionAllow, decision.Key)
	assert.Equal(t, "because", decision.Reason)
	assert.Equal(t, []byte("body"), decision.Content)
}

// A choice with no options at all is a free-form prompt: the option id IS the
// answer, passed through untouched.
func TestDecide_PassesTheRawIDThroughForAnOptionlessChoice(t *testing.T) {
	t.Parallel()

	decision, err := answerdesk.Decide(domain.ActivityChoice{ID: "choice-1"}, []string{"typed text"}, "", nil)

	require.NoError(t, err)
	assert.Equal(t, "typed text", decision.Key)
	assert.Nil(t, decision.Answers)
}

func TestDecide_MapsAnAnswerKindBackOntoItsQuestions(t *testing.T) {
	t.Parallel()

	choice := domain.ActivityChoice{
		ID: "choice-1",
		Questions: []domain.ActivityChoiceQuestion{
			{
				ID: "q-one", Title: "Pick one",
				Options: []domain.ActivityChoiceOption{
					{ID: "q-one-a", Kind: domain.ChoiceOptionAnswer, Label: "Alpha"},
				},
			},
			{
				ID: "q-many", Title: "Pick many", Multi: true,
				Options: []domain.ActivityChoiceOption{
					{ID: "q-many-a", Kind: domain.ChoiceOptionAnswer, Label: "Beta"},
					{ID: "q-many-b", Kind: domain.ChoiceOptionAnswer, Label: "Gamma"},
				},
			},
		},
	}

	decision, err := answerdesk.Decide(choice, []string{"q-one-a", "q-many-a", "q-many-b"}, "", nil)

	require.NoError(t, err)
	require.Equal(t, domain.ChoiceOptionAnswer, decision.Key)
	require.Len(t, decision.Answers, 2)
	assert.Equal(t, "Alpha", decision.Answers["Pick one"], "a single-pick question answers with the label itself")
	assert.Equal(t, []any{"Beta", "Gamma"}, decision.Answers["Pick many"], "a multi-pick question answers with a list")
}
