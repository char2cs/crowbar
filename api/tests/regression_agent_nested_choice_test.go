//go:build integration

package tests

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nestedChoiceDescriptorYAML is the answer stub plus the two tool events a
// provider that streams a subagent's own thread over the parent's connection
// uses: tool_post naming the child it spawned (spawns_scope), then the child's own
// events carrying the child's session id.
const nestedChoiceEventsYAML = `  tool_pre:
    in: tool_pre
    map:
      session_id: session_id
      tool_id: tool_id
      tool_name: tool_name
  tool_post:
    in: tool_post
    kind_map: { spawn: subagent }
    map:
      session_id: session_id
      tool_id: tool_id
      tool_name: tool_name
      tool_kind: tool_name
      spawns_scope: spawns_scope
`

func nestedChoiceDescriptor() string {
	return strings.Replace(answerStubProviderDescriptorYAML, "runtime:\n", nestedChoiceEventsYAML+"runtime:\n", 1)
}

type nestedChoiceChat struct {
	choiceFrameChat
}

func newNestedChoiceChat(t *testing.T) nestedChoiceChat {
	t.Helper()
	c := newChoiceFrameChat(t, nestedChoiceDescriptor())
	hook := func(event, payload string) {
		postHook(t, c.h, repoBase(c.imported), "answerstub", c.runnerID, event, payload)
	}
	hook("session_start", `{"session_id":"main"}`)
	hook("tool_post", `{"session_id":"main","tool_id":"spawn-1","tool_name":"spawn","spawns_scope":"child"}`)
	c.h.Quiesce()
	return nestedChoiceChat{c}
}

func (c nestedChoiceChat) nestedAsk(t *testing.T, promptID string) (choiceID string) {
	t.Helper()
	// A nested ask is held for the person, not left to the provider's terminal.
	return c.askAs(t.Context(), t, "child", promptID)
}

func TestRegression_ANestedConversationsPermissionBecomesAnAnswerableCard(t *testing.T) {
	c := newNestedChoiceChat(t)
	conn := c.dial()

	choiceID := c.nestedAsk(t, "np1")
	readUntil(t, conn, c.choiceFrame(func(choices []map[string]any) bool {
		return len(choices) == 1 && choices[0]["id"] == choiceID && choices[0]["answerable"] == true
	}))

	// A redelivery of the same ask must not mint a second card.
	c.h.Quiesce()
	var listed []struct {
		ID string `json:"id"`
	}
	c.h.get(repoBase(c.imported)+"/chats/"+c.chatID+"/choices", &listed)
	require.Len(t, listed, 1)

	c.answer(t, choiceID)
	readUntil(t, conn, c.choiceFrame(func(choices []map[string]any) bool { return len(choices) == 0 }))

	assert.JSONEq(t, `{"decision":{"behavior":"allow"}}`, c.reply(t, choiceID),
		"the reply returns to the nested request's own ask")
}

func TestRegression_ANestedAskDoesNotChangeHowNestedToolRowsRender(t *testing.T) {
	c := newNestedChoiceChat(t)
	postHook(t, c.h, repoBase(c.imported), "answerstub", c.runnerID, "tool_pre",
		`{"session_id":"child","tool_id":"ct1","tool_name":"Bash"}`)
	c.nestedAsk(t, "np1")
	postHook(t, c.h, repoBase(c.imported), "answerstub", c.runnerID, "tool_post",
		`{"session_id":"child","tool_id":"ct1","tool_name":"Bash"}`)
	c.h.Quiesce()

	calls := channelSplitActivity(t, c.h, c.imported, c.chatID)
	ids := make([]string, 0, len(calls))
	for _, call := range calls {
		ids = append(ids, call.ID)
	}
	assert.Contains(t, ids, "ct1", "the child's own tool call is still recorded")
	assert.Contains(t, ids, "spawn-1")
	assert.Len(t, ids, 2)
}

// A subagent's CLI held on an ask past its turn's end really waits, so the ask
// is a card the person answers, as a held top-level ask outside a turn is.
func TestRegression_ANestedAskAfterTheTurnClosedIsAnAnswerableCard(t *testing.T) {
	c := newNestedChoiceChat(t)
	conn := c.dial()
	postHook(t, c.h, repoBase(c.imported), "answerstub", c.runnerID, "turn_stop",
		`{"session_id":"main","last_assistant_message":"done"}`)
	c.h.Quiesce()

	choiceID := c.nestedAsk(t, "late")
	readUntil(t, conn, c.choiceFrame(func(choices []map[string]any) bool {
		return len(choices) == 1 && choices[0]["id"] == choiceID && choices[0]["answerable"] == true
	}))

	c.answer(t, choiceID)
	assert.JSONEq(t, `{"decision":{"behavior":"allow"}}`, c.reply(t, choiceID),
		"the person's answer reaches the held subagent")
}
