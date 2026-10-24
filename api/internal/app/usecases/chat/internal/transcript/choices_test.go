package transcript_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/transcript"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func asked(t *testing.T, a ledger.Ask) ledger.Row {
	t.Helper()
	return mk(t, ledger.KindAsk, "T1", "", a)
}

func TestChoices_OnePerAskWithItsAnswerable(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	options, err := json.Marshal([]domain.ActivityChoiceOption{{ID: "allow", Label: "Allow"}})
	require.NoError(t, err)
	b.add(
		asked(t, ledger.Ask{
			AskID: "choice-1", Event: "permission", Kind: domain.ChoiceKindPermission, ToolName: "Bash",
			Options: options, Schema: json.RawMessage(`{"type":"object"}`), Answerable: true,
		}),
		asked(t, ledger.Ask{
			AskID: "choice-2", Event: "permission", Kind: domain.ChoiceKindQuestion,
			Questions: json.RawMessage(`"not a list"`),
		}),
		asked(t, ledger.Ask{AskID: "choice-3", Event: "notification"}),
		asked(t, ledger.Ask{AskID: "choice-1", Event: "permission", Kind: domain.ChoiceKindPermission}),
	)

	choices := b.activity().Choices
	require.Len(t, choices, 2, "one per ask that offers a choice")
	assert.True(t, choices[0].Answerable)
	assert.Equal(t, "Bash", choices[0].ToolName)
	assert.Equal(t, []domain.ActivityChoiceOption{{ID: "allow", Label: "Allow"}}, choices[0].Options)
	assert.JSONEq(t, `{"type":"object"}`, choices[0].Schema)
	assert.False(t, choices[1].Answerable)
	assert.Nil(t, choices[1].Questions, "what does not decode is left out, the question kept")

	pending, err := b.p.PendingChoices(t.Context(), chatID)
	require.NoError(t, err)
	require.Len(t, pending, 2)
}

func TestChoices_AnAnsweredAskCarriesWhatWasChosen(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.add(asked(t, ledger.Ask{AskID: "choice-1", Event: "permission", Kind: domain.ChoiceKindPermission, Answerable: true}))
	b.add(
		mk(t, ledger.KindAskResolved, "T1", "", ledger.AskResolved{
			AskID: "choice-1", Resolution: ledger.ResolutionAnswered, OptionIDs: []string{"allow"},
		}),
		mk(t, ledger.KindAskResolved, "T1", "", ledger.AskResolved{AskID: "choice-1", Resolution: ledger.ResolutionAbandoned}),
	)

	c := b.activity().Choices[0]
	assert.Equal(t, ledger.ResolutionAnswered, c.Resolution, "an ask closes once")
	assert.Equal(t, []string{"allow"}, c.AnsweredOptionIDs)
	assert.False(t, c.Answerable, "a closed ask is answered no more")
	pending, err := b.p.PendingChoices(t.Context(), chatID)
	require.NoError(t, err)
	assert.Empty(t, pending)
}

func TestChoices_AnAskItsTurnOutlivedIsAbandoned(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.add(asked(t, ledger.Ask{AskID: "choice-1", Event: "permission", Kind: domain.ChoiceKindPermission}))
	b.end()

	c := b.activity().Choices[0]
	assert.Equal(t, ledger.ResolutionAbandoned, c.Resolution)
	assert.Empty(t, c.AnsweredOptionIDs)
}

// What a publication announces is read from the state alone, under the writer.
func TestChoices_PendingChoicesOfTheState(t *testing.T) {
	b := newBook(t)
	b.open("T1", "go")
	b.add(
		asked(t, ledger.Ask{AskID: "choice-1", Event: "permission", Kind: domain.ChoiceKindPermission}),
		asked(t, ledger.Ask{AskID: "choice-2", Event: "notification"}),
	)
	st, err := b.ledger.State(t.Context(), chatID)
	require.NoError(t, err)

	got := transcript.PendingChoicesOf(st)
	require.Len(t, got, 1)
	assert.Equal(t, "choice-1", got[0].ID)
	assert.Equal(t, chatID, got[0].ChatID)
	assert.Equal(t, "interrupt-1", transcript.InterruptionID("choice-1"))
}
