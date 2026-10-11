//go:build integration

package tests

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// dial opens a WebSocket to the given /v0 path and registers cleanup.
func (h *harness) dial(
	path string,
) *websocket.Conn {
	h.t.Helper()
	wsURL := "ws" + h.url[len("http"):] + path
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(h.t, err)
	h.t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// readUntil blocks reading JSON frames, skipping control and non-JSON frames,
// until match returns true for a decoded frame, and returns that frame.
//
// It carries NO read deadline, by design. The frame's arrival IS the signal, so
// the read simply blocks until the daemon produces it. A deadline here would
// only restate `go test -timeout` — and restate it worse: an expired deadline
// surfaces as a bare "i/o timeout" on some assertion, telling you nothing about
// which frame never came, and it is the construct that goes flaky under load.
//
// A frame that genuinely never arrives is therefore a HANG, resolved by
// `go test -timeout` (the suite's only backstop). That is the better failure:
// the timeout panics with a full goroutine dump in which this read is parked in
// conn.ReadMessage, and the stack immediately above it names the calling test
// and the exact readUntil call site — i.e. it points straight at the frame that
// never came. The only assertion left here is a real one: a read ERROR, meaning
// the connection broke before the awaited frame arrived.
func readUntil(
	t *testing.T,
	conn *websocket.Conn,
	match func(map[string]any) bool,
) map[string]any {
	t.Helper()
	// A FAILURE BOUND, not a wait. The awaited frame either arrives promptly or
	// it is never coming: this helper's whole failure mode is a frame broadcast
	// BEFORE the caller subscribed, and no amount of further blocking recovers
	// it. Unbounded, that costs the entire PACKAGE — Go's 4m timeout kills every
	// other test in ./tests and leaves a goroutine dump as the only evidence of
	// which one was at fault. Bounded, the test names itself instead.
	//
	// 45s, not 30s: TestRegression_DeleteWorkspaceTombstones was seen missing
	// this bound under CI's constrained CPU with an otherwise clean, leak-free
	// harness — worst-case scheduling latency against a large legitimate
	// goroutine footprint, not a hang. Still an order of magnitude under the
	// package's own 4m ceiling.
	deadline := time.Now().Add(45 * time.Second)
	for {
		require.NoError(t, conn.SetReadDeadline(deadline))
		mt, raw, err := conn.ReadMessage()
		require.NoError(t, err, "no matching frame within 30s — it was most "+
			"likely broadcast before this connection subscribed")
		if mt != websocket.TextMessage {
			continue
		}
		var got map[string]any
		if json.Unmarshal(raw, &got) != nil {
			continue
		}
		if match(got) {
			return got
		}
	}
}

// sentinelFile is the file a test dirties in a watched worktree to push a frame
// of its OWN through a Git topic. It is the barrier used to prove NEGATIVES on a
// WS topic without a clock: no fixture ever creates or touches it, so a status
// frame listing it is unambiguously the test's own, and its arrival proves that
// every frame the observed window was ever going to produce has already been
// delivered on that connection.
const sentinelFile = "crowbar-sentinel.txt"

// readTextFrame blocks until the next TEXT frame and returns its raw bytes,
// skipping control frames. Like readUntil it carries no deadline: the frame's
// arrival is the signal. Use it (rather than readUntil) when the test must
// COUNT or inspect every frame, including the ones it does not match on.
func readTextFrame(
	t *testing.T,
	conn *websocket.Conn,
) []byte {
	t.Helper()
	for {
		mt, raw, err := conn.ReadMessage()
		require.NoError(t, err, "ws closed before the awaited frame arrived")
		if mt == websocket.TextMessage {
			return raw
		}
	}
}

// statusHasFile reports whether a decoded GitStatus frame lists name among its
// dirty files. The Git broadcaster filters on wsId but serialises a BARE
// GitStatus (no wsId on the wire), so a frame's dirty-file set is the only
// discriminator a WS test has for which workspace produced it.
func statusHasFile(
	frame map[string]any,
	name string,
) bool {
	files, ok := frame["files"].([]any)
	if !ok {
		return false
	}
	for _, f := range files {
		entry, ok := f.(map[string]any)
		if ok && entry["path"] == name {
			return true
		}
	}
	return false
}

// waitForWorkComplete blocks until wsID's in-flight async mutation has STARTED and
// then FINISHED. It is the completion barrier for the fire-and-forget 202 handlers,
// whose slow work (provisioning a worktree, running a merge) happens in a detached
// goroutine that no projection drain can observe.
//
// It reads the daemon's OWN "am I busy" overlay: every detached op is bracketed
// BeginWork → EndWork, and EndWork fires only after the work function has returned,
// on success, error and panic alike. Both edges broadcast a WorkspaceDTO, so the
// falling edge is the daemon stating the op is over. Waiting for the RISING edge
// first is what makes the falling one mean anything — a workspace at rest already
// reads working:false, so matching that alone would return before the op began.
//
// Mirrors kit.WaitForWorkComplete for the httptest-based suites in this package.
func waitForWorkComplete(
	t *testing.T,
	conn *websocket.Conn,
	wsID string,
) {
	t.Helper()
	isWorking := func(m map[string]any, want bool) bool {
		if m["id"] != wsID {
			return false
		}
		working, ok := m["working"].(bool)
		return ok && working == want
	}
	readUntilWorktree(t, conn, func(m map[string]any) bool { return isWorking(m, true) })
	readUntilWorktree(t, conn, func(m map[string]any) bool { return isWorking(m, false) })
}

// worktreeFrame projects one chat-feed frame down to the flat workspace map the
// deleted `workspaces` stream used to send, so a predicate written against that
// stream reads the same keys off this one.
//
// It is an UNWRAP, not a translation. A worktree_state frame is the workspace's
// own WorkspaceDTO put through dto.ChatWorktreeFrom and hung under `worktree`
// (v0.Container.pushChatWorktree), so every key here is the byte the workspace
// stream would have sent — this only lifts them back out of the envelope and
// restores the two identity fields the envelope, not the payload, carries: `id`
// (the frame's workspaceId) and `repoId`.
//
// projectId, createdAt and kind are NOT recoverable: the chat feed never carried
// them.
//
// A frame that is not a worktree_state one is returned UNCHANGED, which is what
// keeps readUntilWorktree usable on the flat DTO streams (projects, repos,
// threads) as well as this one.
func worktreeFrame(
	m map[string]any,
) map[string]any {
	wt, ok := m["worktree"].(map[string]any)
	if !ok {
		return m
	}
	wsID, _ := m["workspaceId"].(string)
	if wsID == "" {
		return m
	}
	out := make(map[string]any, len(wt)+3)
	for k, v := range wt {
		out[k] = v
	}
	out["id"] = wsID
	out["chatId"] = m["chatId"]
	if repoID, ok := m["repoId"]; ok {
		out["repoId"] = repoID
	}
	return out
}

// readUntilWorktree is readUntil with worktreeFrame applied to every frame
// before match sees it, returning the PROJECTED map. Use it on the chat feed
// (.../chats/ws or /v0/chats/:chatId/ws) wherever a predicate is written in the
// old workspace vocabulary.
func readUntilWorktree(
	t *testing.T,
	conn *websocket.Conn,
	match func(map[string]any) bool,
) map[string]any {
	t.Helper()
	var hit map[string]any
	readUntil(t, conn, func(raw map[string]any) bool {
		m := worktreeFrame(raw)
		if match(m) {
			hit = m
			return true
		}
		return false
	})
	return hit
}
