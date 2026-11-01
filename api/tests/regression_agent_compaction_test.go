//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// compactableStubProviderDescriptorYAML declares compact_pre/compact_post —
// streamStubProviderDescriptorYAML (regression_agent_message_stream_test.go)
// does not, since the message-stream tests never need them. The map keys
// mirror claude.yaml's own compact_pre/compact_post exactly (session_id,
// trigger).
const compactableStubProviderDescriptorYAML = `id: compactablestub
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
  compact_pre:
    in: compact_pre
    map:
      session_id: session_id
      trigger: trigger
  compact_post:
    in: compact_post
    map:
      session_id: session_id
      trigger: trigger
runtime:
  transport: hooks
  hooks:
    format: json
`

// The compaction hooks are recorded as markers on the chat's feed, and a turn
// after compact_post starts as usual.
func TestRegression_CompactPreAndPostAreRecordedOnTheFeed(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "compactablestub", compactableStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "compactablestub")

	post := func(event, payload string) {
		postHook(t, h, repoBase(imported), "compactablestub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("compact_pre", `{"session_id":"sess-1","trigger":"manual"}`)
	post("compact_post", `{"session_id":"sess-1","trigger":"manual"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"hello"}`)

	rows := chatRows(t, h, chatID)
	marker := func(kind string) func(ledger.Marker) bool {
		return func(b ledger.Marker) bool { return b.Kind == kind }
	}
	started := seqOf(t, rows, ledger.KindMarker, marker(ledger.MarkerCompactionStarted))
	ended := seqOf(t, rows, ledger.KindMarker, marker(ledger.MarkerCompactionEnded))
	turn := seqOf(t, rows, ledger.KindTurnStarted, func(ledger.TurnStarted) bool { return true })
	assert.Less(t, started, ended)
	assert.Less(t, ended, turn, "compact_post leaves nothing wedged for the turn after it")
}
