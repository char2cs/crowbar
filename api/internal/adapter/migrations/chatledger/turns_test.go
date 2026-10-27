package chatledger

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func TestConvertChat_HarnessRowsKeepTheirOriginAndNoticesFailTheTurn(t *testing.T) {
	res := convert(t, chatA(), nobody)

	messages := bodies[ledger.UserMessage](t, res, ledger.KindUserMessage)
	harness := messages[3]
	assert.Equal(t, ledger.UserMessage{Text: "harness note", Origin: ledger.OriginHarness}, harness)
	failed := rowsOf(res, ledger.KindTurnFailed)
	require.Len(t, failed, 1)
	assert.Equal(t, ledger.TurnFailed{Reason: reasonNotice, Detail: "usage limit reached"}, body[ledger.TurnFailed](t, failed[0]))
	assert.Equal(t, turnID("chat-a", "h-1"), failed[0].TurnID, "the notice fails the turn it is in")
	last, ok := fold(res).LastTurn()
	require.True(t, ok)
	assert.Equal(t, ledger.TurnStatusFailed, last.Status)
}

func TestConvertChat_AnInferredInterruptionEndsTheTurnThatRecordedNoEnd(t *testing.T) {
	res := convert(t, chatB(), nobody)

	ends := rowsOf(res, ledger.KindTurnEnded)
	require.Len(t, ends, 2)
	assert.Equal(t, turnID("chat-b", "u-b2"), ends[1].TurnID, "the open reply's id names the runner's turn")
	assert.Equal(t, ledger.TurnEnded{Status: "interrupted", Reason: kindInferred, RunnerID: "run-b"}, body[ledger.TurnEnded](t, ends[1]))
	assert.Equal(t, at(9, 11, 0), ends[1].At)
}

func TestConvertChat_AnInferredInterruptionOfATurnThatEndedIsSkipped(t *testing.T) {
	in := chatB()
	in.Interruptions[0].TurnID = "msg-b1"

	res := convert(t, in, nobody)

	assert.Contains(t, res.Skipped, "interruption i-b1: inferred, and its turn has an end")
	ends := bodies[ledger.TurnEnded](t, res, ledger.KindTurnEnded)
	assert.Equal(t, ledger.ReasonDaemonRestart, ends[1].Reason, "the turn with no reply still ends")
}

// A stopped reply the CLI finished after the next prompt keeps its place: the
// next turn begins after it ends.
func TestConvertChat_ATurnBeginsAfterTheOneBeforeItEnds(t *testing.T) {
	in := chatA()
	in.Turns[1].EndedAt = ptr(at(10, 5, 30))

	res := convert(t, in, nobody)

	var turnOfRow []string
	for _, r := range res.Rows {
		if r.Kind == ledger.KindTurnStarted || r.Kind == ledger.KindTurnEnded || r.Kind == ledger.KindTurnFailed {
			turnOfRow = append(turnOfRow, r.TurnID)
		}
	}
	for i := 0; i+1 < len(turnOfRow); i += 2 {
		assert.Equal(t, turnOfRow[i], turnOfRow[i+1], "each turn ends before the next starts")
	}
}

func TestConvertChat_ARowOfAnUnknownRoleIsSkipped(t *testing.T) {
	in := chatB()
	in.Turns = append(in.Turns, LegacyTurn{ID: "x-1", ChatID: in.ChatID, Role: "system", Seq: 9, DisplayOrder: 9, StartedAt: at(9, 20, 0)})

	res := convert(t, in, nobody)

	assert.Contains(t, res.Skipped, `turn x-1: unknown role "system"`)
}

func TestConvertChat_AReplyEndsTheTurnAsItsStatusSays(t *testing.T) {
	for status, want := range map[string]ledger.Kind{
		statusFailed: ledger.KindTurnFailed, statusInterrupted: ledger.KindTurnEnded, statusAbandoned: ledger.KindTurnEnded,
	} {
		in := chatB()
		in.Turns[1].Status = status

		res := convert(t, in, nobody)

		first := res.Rows[slices.IndexFunc(res.Rows, func(r ledger.Row) bool {
			return r.TurnID == turnID("chat-b", "u-b1") && (r.Kind == ledger.KindTurnEnded || r.Kind == ledger.KindTurnFailed)
		})]
		assert.Equal(t, want, first.Kind, status)
		if want == ledger.KindTurnEnded {
			assert.Equal(t, ledger.TurnEnded{Status: "interrupted", Reason: status, RunnerID: "run-b"}, body[ledger.TurnEnded](t, first))
		}
	}
}

// A reply the old daemon kept with no prompt before it (the CLI's own turn)
// is a turn of its own; a notice between turns is what the CLI told its user.
func TestConvertChat_AReplyWithNoPromptIsATurnAndANoticeOutsideOneAMarker(t *testing.T) {
	in := chatB()
	in.Turns = in.Turns[1:2]
	in.Turns = append([]LegacyTurn{{ID: "n-0", ChatID: in.ChatID, Role: roleNotice, Text: "rate limited", StartedAt: at(9, 0, 30)}}, in.Turns...)
	in.Interruptions = nil

	res := convert(t, in, nobody)

	assert.Empty(t, rowsOf(res, ledger.KindUserMessage))
	assert.Equal(t, turnID("chat-b", "msg-b1"), rowsOf(res, ledger.KindTurnStarted)[0].TurnID)
	assert.Equal(t, []ledger.Marker{{Kind: ledger.MarkerNotification, Detail: "rate limited"}}, bodies[ledger.Marker](t, res, ledger.KindMarker))
}

// A reply row the old daemon kept with no id still restates: its message is
// named by its seq, as its turn is.
func TestConvertChat_AReplyWithNoIDIsNamedByItsSeq(t *testing.T) {
	in := chatB()
	in.Turns[1].ID = ""

	res := convert(t, in, nobody)

	assert.Equal(t, "seq-2", bodies[ledger.AssistantText](t, res, ledger.KindAssistantText)[0].MessageID)
}
