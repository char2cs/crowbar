//go:build integration

package tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
	"github.com/char2cs/crowbar/api/tests/kit"
)

type retractAnswer struct {
	State string `json:"state"`
	Text  string `json:"text"`
}

func (c turnChat) retractPath(requestID string) string {
	return repoBase(c.imported) + "/chats/" + c.chatID + "/prompts/" + requestID + "/retract"
}

func (c turnChat) retract(t *testing.T, requestID string) retractAnswer {
	t.Helper()
	var got retractAnswer
	c.h.post(c.retractPath(requestID), nil, http.StatusOK, &got)
	return got
}

// send submits the prompt "one" as requestID and returns the launch that carries it.
func (c turnChat) send(t *testing.T, requestID string) kit.Launch {
	t.Helper()
	require.Equal(t, http.StatusOK, postPrompt(t, c.h, c.imported, c.chatID, "one", requestID))
	c.h.QuiesceReactors()
	return c.fake.AwaitLaunch(t, getAgentChat(t, c.h, repoBase(c.imported), c.chatID).LiveRunnerID)
}

// feedRows is the chat's whole feed replay, in seq order.
func (c turnChat) feedRows(t *testing.T) []stream.Frame {
	t.Helper()
	return chatRows(t, c.h, c.chatID)
}

// chatRows is chatID's whole feed replay, in seq order: what a client reads
// the chat from.
func chatRows(t *testing.T, h *harness, chatID string) []stream.Frame {
	t.Helper()
	h.app.Repositories.QuiesceReactors(t.Context())
	feed := dialFeed(t, h, chatID, 0)
	var rows []stream.Frame
	for f := nextFeedFrame(t, feed); f.T != stream.FrameSynced; f = nextFeedFrame(t, feed) {
		rows = append(rows, f)
	}
	return rows
}

// ledgerRows are chatID's feed rows as the ledger rows they carry.
func ledgerRows(t *testing.T, h *harness, chatID string) []ledger.Row {
	t.Helper()
	frames := chatRows(t, h, chatID)
	rows := make([]ledger.Row, 0, len(frames))
	for _, f := range frames {
		rows = append(rows, *f.Row)
	}
	return rows
}

// chatState is chatID's ledger state, folded from its feed rows.
func chatState(t *testing.T, h *harness, chatID string) ledger.State {
	t.Helper()
	st := ledger.State{ChatID: chatID}
	for _, r := range ledgerRows(t, h, chatID) {
		st.Apply(r)
	}
	return st
}

// seqOf is the seq of the first row of kind that keep accepts.
func seqOf[B any](t *testing.T, rows []stream.Frame, kind ledger.Kind, keep func(B) bool) int64 {
	t.Helper()
	for _, f := range rows {
		var b B
		if f.Row.Kind == kind && json.Unmarshal(f.Row.Body, &b) == nil && keep(b) {
			return f.Row.Seq
		}
	}
	require.Failf(t, "no such row", "no %s row matched", kind)
	return 0
}

// rowsOf decodes the body of every row of kind that keep accepts.
func rowsOf[B any](t *testing.T, rows []stream.Frame, kind ledger.Kind, keep func(B) bool) []B {
	t.Helper()
	var out []B
	for _, f := range rows {
		if f.Row.Kind != kind {
			continue
		}
		var b B
		require.NoError(t, json.Unmarshal(f.Row.Body, &b))
		if keep(b) {
			out = append(out, b)
		}
	}
	return out
}

func settledAs(t *testing.T, rows []stream.Frame, requestID string) []ledger.PromptSettled {
	t.Helper()
	return rowsOf(t, rows, ledger.KindPromptSettled, func(b ledger.PromptSettled) bool { return b.RequestID == requestID })
}

func TestRegression_Symptom5_RetractBeforeDeliveryReturnsTheText(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	r1 := uuid.NewString()
	l := c.send(t, r1)

	assert.Equal(t, retractAnswer{State: "retracted", Text: "one"}, c.retract(t, r1))

	c.feed.awaitRunnerGone(c.chatID, l.RunnerID)
	rows := c.feedRows(t)
	assert.Equal(t, []ledger.PromptSettled{{RequestID: r1, Consumed: false, Reason: ledger.ReasonRetracted}}, settledAs(t, rows, r1))
	exits := rowsOf(t, rows, ledger.KindRunnerExited, func(b ledger.RunnerExited) bool { return b.RunnerID == l.RunnerID })
	require.Len(t, exits, 1, "the CLI launched to carry r1 exits")
	assert.Equal(t, "retracted", exits[0].Reason)
}

func TestRegression_Symptom5_TheNextPromptIsAcceptedAfterARetract(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	r1 := uuid.NewString()
	c.send(t, r1)
	c.retract(t, r1)

	assert.Equal(t, http.StatusOK, postPrompt(t, c.h, c.imported, c.chatID, "two", uuid.NewString()),
		"a retracted prompt never holds the chat busy")
}

func TestRegression_Symptom5_RetractAfterDeliveryInterruptsTheTurn(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	r1 := uuid.NewString()
	l := c.send(t, r1)
	c.hook(t, l, "user_prompt", map[string]any{"prompt": "one", "prompt_id": uuid.NewString()})
	require.True(t, c.working(t), "precondition: r1's turn is running")

	assert.Equal(t, retractAnswer{State: "interrupted", Text: "one"}, c.retract(t, r1))

	assert.False(t, c.working(t), "the turn ends before the retract answers")
	ended := rowsOf(t, c.feedRows(t), ledger.KindTurnEnded, func(b ledger.TurnEnded) bool { return b.Reason == ledger.ReasonRetracted })
	require.Len(t, ended, 1)
	assert.Equal(t, string(ledger.TurnStatusInterrupted), ended[0].Status)
}

func TestRegression_Symptom5_RetractingASettledPromptIs409(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	r1 := uuid.NewString()
	l := c.send(t, r1)
	c.hook(t, l, "user_prompt", map[string]any{"prompt": "one", "prompt_id": "P1"})
	c.hook(t, l, "turn_stop", map[string]any{"prompt_id": "P1", "last_assistant_message": ""})
	require.False(t, c.working(t), "precondition: r1's turn is over")

	status, _ := answerOf(t, c.h, c.retractPath(r1), nil)
	assert.Equal(t, http.StatusConflict, status, "a prompt whose turn closed cannot be withdrawn")
	status, _ = answerOf(t, c.h, c.retractPath(uuid.NewString()), nil)
	assert.Equal(t, http.StatusNotFound, status, "nothing was sent under an unknown id")
}

func TestRegression_Symptom5_ALateAckAfterRetractIsRecordedAsAProviderTurn(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	c.startTurn(t, c.launch, "A")
	r1 := uuid.NewString()
	require.Equal(t, http.StatusOK, postPrompt(t, c.h, c.imported, c.chatID, "late text", r1), "steered into turn A")
	require.Equal(t, retractAnswer{State: "retracted", Text: "late text"}, c.retract(t, r1))

	c.hook(t, c.launch, "turn_stop", map[string]any{"prompt_id": "A", "last_assistant_message": ""})
	c.hook(t, c.launch, "user_prompt", map[string]any{"prompt": "late text", "prompt_id": "B"})

	assert.True(t, c.working(t), "the provider's own turn runs")
	rows := c.feedRows(t)
	late := rowsOf(t, rows, ledger.KindUserMessage, func(b ledger.UserMessage) bool { return b.Text == "late text" })
	require.Len(t, late, 2, "r1's row, then the provider's")
	assert.Equal(t, ledger.OriginCrowbar, late[0].Origin)
	assert.Equal(t, ledger.OriginProvider, late[1].Origin)
	assert.Empty(t, late[1].RequestID, "the late ack is not r1")
	assert.Equal(t, []ledger.PromptSettled{{RequestID: r1, Reason: ledger.ReasonRetracted}}, settledAs(t, rows, r1),
		"r1 stays retracted")
	assert.Equal(t, ledger.KindTurnStarted, rows[len(rows)-1].Row.Kind, "the provider's prompt opened its turn")
}
