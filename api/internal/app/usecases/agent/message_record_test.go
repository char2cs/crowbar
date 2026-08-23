package agent_test

import (
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deltaCallbackRecorder captures every call the usecase makes on the live
// message-delta callback it was handed at sweep start.
type deltaCallbackRecorder struct {
	mu    sync.Mutex
	calls []deltaCall
}

type deltaCall struct {
	chatID      string
	workspaceID string
	messageID   string
	text        string
}

func (r *deltaCallbackRecorder) record(chatID, workspaceID, messageID, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, deltaCall{chatID, workspaceID, messageID, text})
}

func (r *deltaCallbackRecorder) snapshot() []deltaCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]deltaCall(nil), r.calls...)
}

func (r *deltaCallbackRecorder) texts() []string {
	out := []string{}
	for _, c := range r.snapshot() {
		out = append(out, c.text)
	}
	return out
}

// deltaHook renders one claude `message_delta` payload exactly as the bundled
// descriptor declares it (session_id/turn_id/message_id/index/final/delta), so
// the real engine parses it rather than a shape invented here.
func deltaHook(t *testing.T, messageID string, index int, final bool, text string) []byte {
	t.Helper()
	return mustJSON(t, map[string]any{
		"session_id": "sess-1",
		"turn_id":    "turn-1",
		"message_id": messageID,
		"index":      index,
		"final":      final,
		"delta":      text,
	})
}

// TestStartTerminalWaitSweep_PushesEveryDeltaAsTheMessageSoFar pins the live
// streaming callback: the thing that puts an assistant message on screen WHILE
// it is being said. It is a plain field on the usecase, assigned at sweep start
// and read on every delta, so nothing about it fails to compile if it stops
// being called — the chat pane simply goes silent until the turn ends.
//
// It asserts the CUMULATIVE text, not the increment. A client that missed one
// frame must be correct again on the next one with no reassembly of its own,
// which is only true if each call carries everything said so far.
//
// No goroutine and no timing: fakeCommander implements no Screen method, so
// newTerminalWaitDetector finds no termwait.Screens, u.termWait stays nil, and
// StartTerminalWaitSweep assigns the callbacks and returns without starting the
// sweep. Every call below therefore lands on this test's own goroutine, inside
// IngestHookDelivery.
func TestStartTerminalWaitSweep_PushesEveryDeltaAsTheMessageSoFar(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	deltas := &deltaCallbackRecorder{}
	f.usecase.StartTerminalWaitSweep(f.ctx, nil, nil, deltas.record)

	post := func(index int, final bool, text string) {
		t.Helper()
		require.NoError(t, f.usecase.IngestHookDelivery(
			f.ctx, "ws1", uuid.NewString(), runnerID, "claude", "message_delta",
			deltaHook(t, "msg-one", index, final, text),
		))
	}
	post(0, false, "THE ")
	post(1, false, "MESSAGE ")
	post(2, true, "SO FAR")
	f.wait()

	assert.Equal(t,
		[]string{"THE ", "THE MESSAGE ", "THE MESSAGE SO FAR"},
		deltas.texts(),
		"every delta must push the message SO FAR, so a client that dropped a frame is correct on the next one")

	for _, call := range deltas.snapshot() {
		assert.Equal(t, chatID, call.chatID, "the frame must name the chat it belongs to")
		assert.Equal(t, "ws1", call.workspaceID, "the feed is workspace-scoped")
		assert.Equal(t, "msg-one", call.messageID,
			"the provider's own message identity is how a client tells a growing message from the next one")
	}

	// The partials exist ONLY on this callback. Nothing durable ever held "THE "
	// or "THE MESSAGE ", so a build that stopped calling it would leave the pane
	// with nothing to show until the turn ended — and the ledger below would
	// still read exactly the same.
	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 100)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "THE MESSAGE SO FAR", page.Items[0].Text)
}
