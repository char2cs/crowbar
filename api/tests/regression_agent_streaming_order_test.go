//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// Text said before a tool ran is recorded before it, so a client draws the
// tool below the text that preceded it.
func TestRegression_AStreamingMessageIsRecordedBeforeTheToolThatFollowsIt(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "streamstub")

	post := func(event, payload string) {
		postHook(t, h, repoBase(imported), "streamstub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"speak then work"}`)
	post("message_delta", delta("msg-one", 0, false, "ABOUT TO RUN "))
	h.Quiesce()
	post("tool_pre", `{"session_id":"sess-1","tool_use_id":"tool-1","tool_name":"Bash",`+
		`"tool_input":{"command":"ls"}}`)
	h.Quiesce()
	post("message_delta", delta("msg-one", 1, false, "A COMMAND"))

	rows := chatRows(t, h, chatID)
	said := seqOf(t, rows, ledger.KindAssistantText, func(b ledger.AssistantText) bool { return b.MessageID == "msg-one" })
	ran := seqOf(t, rows, ledger.KindToolCall, func(b ledger.ToolCall) bool { return b.ToolID == "tool-1" })
	assert.Less(t, said, ran, "text that began before the tool ran must be recorded before it")
}
