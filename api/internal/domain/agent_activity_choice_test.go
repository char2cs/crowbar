package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ResolvePicks is the ONE rule two layers enforce — the usecase that renders an
// answer into a provider's JSON, and the aggregate that records the decision.
// They used to write it twice and disagree, and neither of them noticed a partial
// answer to a multi-question prompt. It is unit-tested here, once, because that is
// where it now lives.

func threeQuestions() domain.ActivityChoice {
	return domain.ActivityChoice{
		ID: "c1", Kind: domain.ChoiceKindQuestion,
		Questions: []domain.ActivityChoiceQuestion{
			{ID: "q0", Text: "Which language?", Options: []domain.ActivityChoiceOption{
				{ID: "q0-answer-0", Kind: domain.ChoiceOptionAnswer, Label: "Go"},
				{ID: "q0-answer-1", Kind: domain.ChoiceOptionAnswer, Label: "TypeScript"},
			}},
			{
				ID: "q1", Text: "Which databases?", Multi: true,
				Options: []domain.ActivityChoiceOption{
					{ID: "q1-answer-0", Kind: domain.ChoiceOptionAnswer, Label: "SQLite"},
					{ID: "q1-answer-1", Kind: domain.ChoiceOptionAnswer, Label: "Redis"},
				},
			},
			{ID: "q2", Text: "Deploy where?", Options: []domain.ActivityChoiceOption{
				{ID: "q2-answer-0", Kind: domain.ChoiceOptionAnswer, Label: "Local"},
			}},
		},
	}
}

func permission() domain.ActivityChoice {
	return domain.ActivityChoice{
		ID: "c1", Kind: domain.ChoiceKindPermission, Title: "Bash",
		Options: []domain.ActivityChoiceOption{
			{ID: "allow", Kind: domain.ChoiceOptionAllow, Label: "Allow"},
			{ID: "deny", Kind: domain.ChoiceOptionDeny, Label: "Deny"},
		},
	}
}

func TestResolvePicks_GroupsAFlatPickListBackOntoItsQuestions(t *testing.T) {
	answers, err := threeQuestions().ResolvePicks([]string{
		"q1-answer-1", "q0-answer-0", "q1-answer-0", "q2-answer-0",
	})

	require.NoError(t, err)
	require.Len(t, answers, 3, "one answer per question, in the order they were asked")
	assert.Equal(t, "Which language?", answers[0].Question.Text)
	assert.Equal(t, []string{"Go"}, labelsOf(answers[0].Picked))
	assert.Equal(t, []string{"Redis", "SQLite"}, labelsOf(answers[1].Picked),
		"a multi-select question keeps the order the human picked in")
	assert.Equal(t, []string{"Local"}, labelsOf(answers[2].Picked))
}

// THE rule. A partial answer is what stranded a live agent: claude was handed
// picks for one of three questions and went on asking for the other two, which
// nothing could send.
func TestResolvePicks_RefusesAnAnswerThatLeavesAnyQuestionUnanswered(t *testing.T) {
	_, err := threeQuestions().ResolvePicks([]string{"q0-answer-0", "q1-answer-0"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "every question must be answered")
	assert.Contains(t, err.Error(), "Deploy where?",
		"with several questions the refusal must say WHICH one is missing")
}

func TestResolvePicks_RefusesTwoPicksOnASingleAnswerQuestion(t *testing.T) {
	_, err := threeQuestions().ResolvePicks([]string{
		"q0-answer-0", "q0-answer-1", "q1-answer-0", "q2-answer-0",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), `"Which language?" takes one answer, got 2`)
}

func TestResolvePicks_RefusesAnIdNoQuestionOffers(t *testing.T) {
	_, err := threeQuestions().ResolvePicks([]string{"q9-answer-0"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not an option on this prompt")
}

// A repeat is not a second answer, it is the same one twice — and on a
// multi-select question it would duplicate a label in the document the provider
// reads.
func TestResolvePicks_RefusesTheSameOptionTwice(t *testing.T) {
	_, err := threeQuestions().ResolvePicks([]string{
		"q0-answer-0", "q1-answer-0", "q1-answer-0", "q2-answer-0",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "picked more than once")
}

// A permission is one question with one answer, so the same rule covers it — and
// "allow AND deny" is refused by the very same clause that refuses two answers to
// a question.
func TestResolvePicks_TreatsAPermissionAsOneQuestion(t *testing.T) {
	answers, err := permission().ResolvePicks([]string{"allow"})
	require.NoError(t, err)
	require.Len(t, answers, 1)
	assert.Equal(t, domain.ChoiceOptionAllow, answers[0].Picked[0].Kind)

	_, err = permission().ResolvePicks([]string{"allow", "deny"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "this prompt takes one answer",
		"a permission's Title is the TOOL's name, and must not be quoted as a question")
}

// A prompt recorded before questions were modelled is a single question described
// by the prompt's own text and options. That is a graceful fallback, not a
// migration: such a row answers exactly as it always did.
func TestResolvePicks_ReadsAPromptRecordedBeforeQuestionsExisted(t *testing.T) {
	legacy := domain.ActivityChoice{
		Kind: domain.ChoiceKindQuestion, Question: "Which do you want?", Multi: true,
		Options: []domain.ActivityChoiceOption{
			{ID: "answer-0", Kind: domain.ChoiceOptionAnswer, Label: "A"},
			{ID: "answer-1", Kind: domain.ChoiceOptionAnswer, Label: "B"},
		},
	}

	answers, err := legacy.ResolvePicks([]string{"answer-0", "answer-1"})

	require.NoError(t, err)
	require.Len(t, answers, 1)
	assert.Equal(t, "Which do you want?", answers[0].Question.AnswerKey())
	assert.Equal(t, []string{"A", "B"}, labelsOf(answers[0].Picked))
}

// An elicitation's answer is a FORM and its ids are the MCP verbs, so there is
// nothing to check them against — and saying so is not the same as refusing.
func TestResolvePicks_HasNothingToSayAboutAPromptWithNoOptions(t *testing.T) {
	elicitation := domain.ActivityChoice{Kind: domain.ChoiceKindElicitation, Question: "details?"}

	answers, err := elicitation.ResolvePicks([]string{"accept"})

	require.NoError(t, err)
	assert.Empty(t, answers)
	assert.Empty(t, elicitation.AskedQuestions())
}

// A question the provider sent neither text nor header for cannot be keyed at
// all, and a refusal about it has nothing to quote.
func TestResolvePicks_NamesAnUntitledQuestionAsThePromptItself(t *testing.T) {
	nameless := domain.ActivityChoice{
		Kind: domain.ChoiceKindQuestion,
		Questions: []domain.ActivityChoiceQuestion{
			{ID: "q0", Options: []domain.ActivityChoiceOption{{ID: "a", Kind: "answer"}}},
			{
				ID: "q1", Title: "Storage",
				Options: []domain.ActivityChoiceOption{{ID: "b", Kind: "answer"}},
			},
		},
	}

	_, err := nameless.ResolvePicks([]string{"b"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "this prompt has no answer")

	assert.Empty(t, nameless.Questions[0].AnswerKey())
	assert.Equal(t, "Storage", nameless.Questions[1].AnswerKey(),
		"the header is the key when the provider sent no question text")

	_, err = nameless.ResolvePicks([]string{"a"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"Storage" has no answer`)
}

func labelsOf(options []domain.ActivityChoiceOption) []string {
	out := make([]string, 0, len(options))
	for _, option := range options {
		out = append(out, option.Label)
	}
	return out
}
