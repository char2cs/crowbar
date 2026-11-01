//go:build integration

package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// A prompt the agent is blocked on is an ask row on the chat's feed, and its
// answer an ask_resolved row.

const answerStubProviderDescriptorYAML = `id: answerstub
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  user_prompt:
    in: user_prompt
    map:
      message: prompt
  turn_stop:
    in: turn_stop
    map:
      session_id: session_id
      message: last_assistant_message
  permission:
    ask: permission
    timeout_seconds: 5
    map:
      session_id: session_id
      prompt_id:  prompt_id
      message:    tool_name
      tool_name:  tool_name
      tool_target: tool_input.command
      tool_input: tool_input
    reply:
      allow: '{"decision":{"behavior":"allow"}}'
      deny: '{"decision":{"behavior":"deny","message":{reason_json}}}'
runtime:
  transport: hooks
  hooks:
    format: json
`

type choiceFrameChat struct {
	h        *harness
	imported importedRepo
	chatID   string
	runnerID string
	// held is each ask's held hook, by choice id: its CLI's answer arrives on it.
	held map[string]<-chan heldHook
}

func newChoiceFrameChat(t *testing.T, descriptor string) choiceFrameChat {
	t.Helper()
	h := newHarness(t)
	writeProviderDescriptor(t, h, "answerstub", descriptor)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "answerstub")
	postHook(t, h, repoBase(imported), "answerstub", runnerID, "user_prompt", `{"prompt":"go"}`)
	h.Quiesce()
	return choiceFrameChat{h: h, imported: imported, chatID: chatID, runnerID: runnerID, held: map[string]<-chan heldHook{}}
}

func (c choiceFrameChat) ask(t *testing.T, promptID string) (choiceID string) {
	t.Helper()
	return c.askAs(t.Context(), t, "s1", promptID)
}

// askAs opens a permission ask from session; the daemon holds its hook open
// until it is decided, or until ctx hangs it up.
func (c choiceFrameChat) askAs(ctx context.Context, t *testing.T, session, promptID string) (choiceID string) {
	t.Helper()
	seen := make([]string, 0, len(c.held))
	for id := range c.held {
		seen = append(seen, id)
	}
	held := askHook(ctx, t, c.h, repoBase(c.imported), "answerstub", c.runnerID, "permission",
		`{"session_id":"`+session+`","prompt_id":"`+promptID+`","tool_name":"Bash",`+
			`"tool_input":{"command":"touch `+promptID+`"}}`)
	choiceID = heldChoice(t, c.h, c.chatID, seen...) //nolint:contextcheck // ctx hangs up the hook only; the feed read has its own deadline
	c.held[choiceID] = held
	return choiceID
}

// reply is what the CLI held on choiceID was answered.
func (c choiceFrameChat) reply(t *testing.T, choiceID string) string {
	t.Helper()
	got := <-c.held[choiceID]
	require.NoError(t, got.err)
	require.Equal(t, http.StatusOK, got.status, got.body)
	return got.body
}

// askState is one ask that offers a choice, as the chat's feed records it.
type askState struct {
	ID, Resolution string
	Answerable     bool
	Options        []struct{ ID, Kind string }
}

// asksOf are chatID's asks that offer a choice, each with its resolution.
func asksOf(t *testing.T, h *harness, chatID string) []askState {
	t.Helper()
	var out []askState
	for _, f := range chatRows(t, h, chatID) {
		switch f.Row.Kind {
		case ledger.KindAsk:
			var b ledger.Ask
			require.NoError(t, json.Unmarshal(f.Row.Body, &b))
			if b.Kind == "" {
				continue
			}
			a := askState{ID: b.AskID, Answerable: b.Answerable}
			if len(b.Options) > 0 {
				require.NoError(t, json.Unmarshal(b.Options, &a.Options))
			}
			out = append(out, a)
		case ledger.KindAskResolved:
			var b ledger.AskResolved
			require.NoError(t, json.Unmarshal(f.Row.Body, &b))
			for i := range out {
				if out[i].ID == b.AskID {
					out[i].Resolution = b.Resolution
				}
			}
		case ledger.KindUserMessage, ledger.KindTurnStarted, ledger.KindPromptSettled, ledger.KindSessionBound,
			ledger.KindSessionLost, ledger.KindMarker, ledger.KindRunnerStarted, ledger.KindRunnerExited,
			ledger.KindTurnEnded, ledger.KindTurnFailed, ledger.KindAssistantText, ledger.KindReasoning,
			ledger.KindToolCall, ledger.KindSubagent, ledger.KindPlan:
			// not an ask
		}
	}
	return out
}

// openAsks are the asks of asks no row resolved.
func openAsks(asks []askState) []askState {
	return slices.DeleteFunc(asks, func(a askState) bool { return a.Resolution != "" })
}

// untilOpen waits for the chat's open asks to satisfy want.
func (c choiceFrameChat) untilOpen(t *testing.T, want func([]askState) bool) {
	t.Helper()
	require.Eventually(t, func() bool { return want(openAsks(asksOf(t, c.h, c.chatID))) },
		10*time.Second, 10*time.Millisecond, "the chat's open asks never matched")
}

func askIDs(asks []askState) []string {
	ids := make([]string, 0, len(asks))
	for _, a := range asks {
		ids = append(ids, a.ID)
	}
	return ids
}

func allowOptionID(t *testing.T, c choiceFrameChat, choiceID string) string {
	t.Helper()
	for _, a := range asksOf(t, c.h, c.chatID) {
		if a.ID != choiceID {
			continue
		}
		for _, option := range a.Options {
			if option.Kind == "allow" {
				return option.ID
			}
		}
	}
	require.FailNow(t, "no allow option for "+choiceID)
	return ""
}

func (c choiceFrameChat) answer(t *testing.T, choiceID string) {
	t.Helper()
	c.h.post(repoBase(c.imported)+"/chats/"+c.chatID+"/choices/"+choiceID+"/answer",
		map[string]any{"optionIds": []string{allowOptionID(t, c, choiceID)}}, http.StatusOK, nil)
}

func TestRegression_AChoiceOpensAndClearsOnTheChatFeed(t *testing.T) {
	c := newChoiceFrameChat(t, answerStubProviderDescriptorYAML)

	choiceID := c.ask(t, "p1")
	c.untilOpen(t, func(asks []askState) bool {
		return len(asks) == 1 && asks[0].ID == choiceID && asks[0].Answerable
	})

	c.answer(t, choiceID)
	c.untilOpen(t, func(asks []askState) bool { return len(asks) == 0 })
}

func TestRegression_SeveralChoicesAreOpenAndEachAnswerClosesOne(t *testing.T) {
	c := newChoiceFrameChat(t, answerStubProviderDescriptorYAML)

	first := c.ask(t, "p1")
	second := c.ask(t, "p2")
	c.untilOpen(t, func(asks []askState) bool { return len(asks) == 2 })

	c.answer(t, first)
	c.untilOpen(t, func(asks []askState) bool {
		ids := askIDs(asks)
		return len(ids) == 1 && ids[0] == second
	})
}

// A CLI that hangs up its held ask has asked the question itself: the choice
// closes as proceeded, and no card is left.
func TestRegression_AHeldAskWhoseCLIHungUpIsClosedAsProceeded(t *testing.T) {
	c := newChoiceFrameChat(t, answerStubProviderDescriptorYAML)

	hangUp, cancel := context.WithCancel(t.Context())
	choiceID := c.askAs(hangUp, t, "s1", "p1")
	c.untilOpen(t, func(asks []askState) bool { return len(asks) == 1 && asks[0].Answerable })

	cancel()
	require.Error(t, (<-c.held[choiceID]).err, "the CLI hung up")
	c.untilOpen(t, func(asks []askState) bool { return len(asks) == 0 })
	all := asksOf(t, c.h, c.chatID)
	require.Len(t, all, 1)
	require.Equal(t, choiceID, all[0].ID)
	require.Equal(t, "proceeded", all[0].Resolution, "its CLI went on without Crowbar's answer")
}

// A held ask nobody answers within its budget is released with nothing to print,
// and its CLI asks the question itself: the choice closes as proceeded.
func TestRegression_AHeldAskWhoseBudgetRanOutIsClosedAsProceeded(t *testing.T) {
	descriptor := strings.Replace(answerStubProviderDescriptorYAML, "timeout_seconds: 5", "timeout_seconds: 1", 1)
	c := newChoiceFrameChat(t, descriptor)

	choiceID := c.ask(t, "p1")
	assert.Empty(t, c.reply(t, choiceID), "an expired hold answers 200 with nothing to print")
	c.untilOpen(t, func(asks []askState) bool { return len(asks) == 0 })
	all := asksOf(t, c.h, c.chatID)
	require.Len(t, all, 1)
	require.Equal(t, "proceeded", all[0].Resolution, "its CLI went on without Crowbar's answer")
}

// The CLI holding an ask receives the bytes Crowbar's record says it was given.
func TestRegression_AnAnsweredAskRepliesOnItsHeldHook(t *testing.T) {
	c := newChoiceFrameChat(t, answerStubProviderDescriptorYAML)
	choiceID := c.ask(t, "p1")

	c.answer(t, choiceID)
	assert.JSONEq(t, `{"decision":{"behavior":"allow"}}`, c.reply(t, choiceID))
}
