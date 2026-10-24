//go:build integration

package tests

import (
	"testing"
)

// compactableStubProviderDescriptorYAML declares compact_pre/compact_post —
// streamStubProviderDescriptorYAML (regression_agent_message_stream_test.go)
// does not, since the message-stream tests never need them. The map keys
// mirror claude.yaml's own compact_pre/compact_post exactly (session_id,
// trigger).
const compactableStubProviderDescriptorYAML = `id: compactablestub
spawn:
  cmd: "cat"
  interactive_required: true
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

// The compaction hooks posted as the CLI fires them push compaction_started
// and compaction_stopped over the socket the working line subscribes to. The
// ledger's compaction markers are a separate record, not this live edge.
func TestRegression_CompactPreAndPostPushLiveOverTheSocket(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "compactablestub", compactableStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)

	conn := h.dial(repoBase(imported) + "/chats/ws")
	chatID, runnerID := createStubChat(t, h, imported, "compactablestub")
	chatFrame := func(kind string) func(map[string]any) bool {
		return func(m map[string]any) bool { return m["chatId"] == chatID && m["kind"] == kind }
	}
	readUntil(t, conn, chatFrame("created"))

	post := func(event, payload string) {
		postProviderHook(t, h, imported, "compactablestub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)

	post("compact_pre", `{"session_id":"sess-1","trigger":"manual"}`)
	readUntil(t, conn, chatFrame("compaction_started"))

	post("compact_post", `{"session_id":"sess-1","trigger":"manual"}`)
	readUntil(t, conn, chatFrame("compaction_stopped"))

	// A real turn afterward proves compact_post's own push did not leave the
	// chat wedged "compacting" for anything that follows it — the backend
	// half is done, cleanly, the instant compact_post lands.
	post("user_prompt", `{"session_id":"sess-1","prompt":"hello"}`)
	readUntil(t, conn, chatFrame("turn_started"))
}

// Both self-heal paths for an unanswered compact_pre (compact_post is
// confirmed unreliable — see agent-chats-slice.ts's own doc comment: "most
// compactions on a small chat never produce one") are FRONTEND concerns:
// "any other chat frame while marked compacting" and the bounded timeout
// backstop are both reducer-side logic in
// use-workspace-agent-chats-stream.ts, covered by that file's own web
// tests. The backend's contract ends at "push compaction_started on
// compact_pre, push compaction_stopped on compact_post" — there is nothing
// further for it to do when compact_post never arrives, so there is no
// corresponding backend-level integration test for that half.
