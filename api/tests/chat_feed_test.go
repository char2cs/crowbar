//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// unnumberedStubProviderDescriptorYAML streams answers with no index: its
// deltas are shown while they happen and no row holds them.
const unnumberedStubProviderDescriptorYAML = `id: unnumberedstub
events:
  message_delta:
    in: message_delta
    map:
      session_id: session_id
      turn_id:    turn_id
      message_id: message_id
      text:       delta
  tool_pre:
    in: tool_pre
    map:
      session_id:  session_id
      tool_id:     tool_use_id
      tool_name:   tool_name
runtime:
  transport: hooks
  hooks:
    format: json
`

// inputStubProviderDescriptorYAML records what a tool call was given.
const inputStubProviderDescriptorYAML = `id: inputstub
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  user_prompt:
    in: user_prompt
    map:
      message: prompt
  tool_pre:
    in: tool_pre
    map:
      session_id: session_id
      tool_id:    tool_use_id
      tool_name:  tool_name
      tool_input: tool_input
runtime:
  transport: hooks
  hooks:
    format: json
`

// feedConn is one chat:<id> hold on its own /v0/stream connection.
type feedConn struct {
	client *streamClient
	scope  string
	epoch  string
}

func dialFeed(t *testing.T, h *harness, chatID string, since int64) *feedConn {
	t.Helper()
	c := dialStream(t, h)
	ready := c.Next(t, isReady)
	fc := &feedConn{client: c, scope: "chat:" + chatID, epoch: ready.Epoch}
	c.Sub(t, fc.scope, since)
	return fc
}

// nextFeedFrame reads the hold's next frame; a frame that never comes fails the test.
func nextFeedFrame(t *testing.T, fc *feedConn) stream.Frame {
	t.Helper()
	return fc.client.Next(t, func(f stream.Frame) bool { return f.Scope == fc.scope })
}

// untilReady reads the replay and returns its row seqs and the synced frame.
func untilReady(t *testing.T, fc *feedConn) ([]int64, stream.Frame) {
	t.Helper()
	var seqs []int64
	for {
		f := nextFeedFrame(t, fc)
		if f.T == stream.FrameSynced {
			return seqs, f
		}
		require.Equal(t, stream.FrameRow, f.T, "only rows come before synced")
		seqs = append(seqs, f.Row.Seq)
	}
}

func contiguous(from, to int64) []int64 {
	var seqs []int64
	for seq := from; seq <= to; seq++ {
		seqs = append(seqs, seq)
	}
	return seqs
}

func newFeedChat(t *testing.T, h *harness, provider, yaml string) (importedRepo, string, func(event, payload string)) {
	t.Helper()
	writeProviderDescriptor(t, h, provider, yaml)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, provider)
	return imported, chatID, func(event, payload string) {
		postHook(t, h, repoBase(imported), provider, runnerID, event, payload)
	}
}

func TestChatFeed_ReplaysFromSinceThenGoesLive(t *testing.T) {
	h := newHarness(t)
	_, chatID, post := newFeedChat(t, h, "streamstub", streamStubProviderDescriptorYAML)
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"hello"}`)

	conn := dialFeed(t, h, chatID, 0)
	seqs, ready := untilReady(t, conn)

	require.Positive(t, ready.Seq)
	assert.Equal(t, contiguous(1, ready.Seq), seqs, "every row, in order, then ready at the head")
	assert.NotEmpty(t, conn.epoch)

	post("message_delta", delta("msg-one", 0, true, "HI"))
	live := nextFeedFrame(t, conn)
	require.Equal(t, stream.FrameRow, live.T)
	assert.Equal(t, ready.Seq+1, live.Row.Seq)
	assert.Equal(t, ledger.KindAssistantText, live.Row.Kind)
}

func TestChatFeed_ResumeSendsOnlyNewerRows(t *testing.T) {
	h := newHarness(t)
	_, chatID, post := newFeedChat(t, h, "streamstub", streamStubProviderDescriptorYAML)
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"hello"}`)
	first := dialFeed(t, h, chatID, 0)
	_, head := untilReady(t, first)
	require.GreaterOrEqual(t, head.Seq, int64(2))

	again := dialFeed(t, h, chatID, head.Seq-2)
	seqs, ready := untilReady(t, again)

	assert.Equal(t, []int64{head.Seq - 1, head.Seq}, seqs)
	assert.Equal(t, head.Seq, ready.Seq)
	assert.Equal(t, first.epoch, again.epoch, "one daemon, one epoch")
}

// An attach landing after a row that holds a stream committed, but before the
// ingress let the stream go, replays the row once and never shows that stream
// again as a live answer. The row is appended past the ingress to hold that
// window open.
func TestChatFeed_AnAttachInsideACommitWindowShowsNoGhostAnswer(t *testing.T) {
	h := newHarness(t)
	_, chatID, post := newFeedChat(t, h, "unnumberedstub", unnumberedStubProviderDescriptorYAML)
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"go"}`)
	say := func(id, text string) {
		post("message_delta", `{"session_id":"sess-1","turn_id":"turn-1","message_id":"`+id+`","delta":"`+text+`"}`)
	}
	say("m-1", "hel")
	say("m-2", "wor")
	chatLedger := h.app.Repositories.ChatLedger
	st, err := chatLedger.State(t.Context(), chatID)
	require.NoError(t, err)
	turn, ok := st.LastTurn()
	require.True(t, ok, "precondition: the turn is streaming")
	whole, err := ledger.NewRow(ledger.KindAssistantText, turn.ID, "",
		ledger.AssistantText{MessageID: "m-1", Index: ledger.IndexComplete, Text: "hello"})
	require.NoError(t, err)
	committed, _, err := chatLedger.Append(t.Context(), chatID, whole)
	require.NoError(t, err)

	conn := dialFeed(t, h, chatID, 0)
	seqs, ready := untilReady(t, conn)

	assert.Equal(t, contiguous(1, ready.Seq), seqs, "each row once, in order")
	assert.Equal(t, committed[0].Seq, ready.Seq, "the replay reaches the row that holds m-1")
	snap := nextFeedFrame(t, conn)
	require.Equal(t, stream.FrameDelta, snap.T)
	assert.Equal(t, "m-2", snap.Delta.ID, "m-1 is its row's now, not a live answer")
	assert.True(t, snap.Delta.Replace)
	assert.Equal(t, "wor", snap.Delta.Text)
	post("tool_pre", `{"session_id":"sess-1","tool_use_id":"tool-1","tool_name":"Bash"}`)
	next := nextFeedFrame(t, conn)
	require.Equal(t, stream.FrameRow, next.T, "the snapshot is m-2 alone")
	assert.Equal(t, ready.Seq+1, next.Row.Seq, "live rows continue from the head")
}

func TestChatFeed_UnnumberedDeltasAreLiveOnly(t *testing.T) {
	h := newHarness(t)
	_, chatID, post := newFeedChat(t, h, "unnumberedstub", unnumberedStubProviderDescriptorYAML)
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"go"}`)
	conn := dialFeed(t, h, chatID, 0)
	_, ready := untilReady(t, conn)
	say := func(text string) {
		post("message_delta", `{"session_id":"sess-1","turn_id":"turn-1","message_id":"m-1","delta":"`+text+`"}`)
	}

	say("a")
	say("b")
	post("tool_pre", `{"session_id":"sess-1","tool_use_id":"tool-1","tool_name":"Bash"}`)

	var deltas []string
	for {
		f := nextFeedFrame(t, conn)
		if f.T == stream.FrameRow {
			assert.Equal(t, ledger.KindToolCall, f.Row.Kind, "no row holds the deltas")
			assert.Equal(t, ready.Seq+1, f.Row.Seq)
			break
		}
		require.Equal(t, stream.FrameDelta, f.T)
		assert.Equal(t, "answer", f.Delta.Stream)
		assert.False(t, f.Delta.Replace)
		deltas = append(deltas, fmt.Sprintf("%d:%s", f.Delta.N, f.Delta.Text))
	}
	assert.Equal(t, []string{"1:a", "2:b"}, deltas, "each frame is only its new text, numbered")

	again := dialFeed(t, h, chatID, 0)
	untilReady(t, again)
	snap := nextFeedFrame(t, again)
	require.Equal(t, stream.FrameDelta, snap.T)
	assert.True(t, snap.Delta.Replace)
	assert.Equal(t, 2, snap.Delta.N)
	assert.Equal(t, "ab", snap.Delta.Text)
	post("tool_pre", `{"session_id":"sess-1","tool_use_id":"tool-2","tool_name":"Bash"}`)
	assert.Equal(t, stream.FrameRow, nextFeedFrame(t, again).T, "the snapshot is one frame per live stream")
}

func TestChatFeed_EpochChangesAcrossARestart(t *testing.T) {
	home := t.TempDir()
	h1 := newHarnessAt(t, home)
	_, chatID, post := newFeedChat(t, h1, "streamstub", streamStubProviderDescriptorYAML)
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"hello"}`)
	first := dialFeed(t, h1, chatID, 0)
	_, before := untilReady(t, first)

	h1.shutdown()
	h2 := newHarnessAt(t, home)
	again := dialFeed(t, h2, chatID, 0)
	seqs, after := untilReady(t, again)

	assert.NotEqual(t, first.epoch, again.epoch, "a restart is a new epoch")
	assert.GreaterOrEqual(t, after.Seq, before.Seq, "seqs continue across it")
	assert.Equal(t, contiguous(1, after.Seq), seqs)
}

func TestChatFeed_DeletingTheChatEndsItsFeed(t *testing.T) {
	h := newHarness(t)
	imported, chatID, _ := newFeedChat(t, h, "streamstub", streamStubProviderDescriptorYAML)
	conn := dialFeed(t, h, chatID, 0)
	untilReady(t, conn)

	_ = h.raw(http.MethodDelete, repoBase(imported)+"/chats/"+chatID, nil, http.StatusOK).Body.Close()

	f := nextFeedFrame(t, conn)
	for f.T != stream.FrameClosed {
		f = nextFeedFrame(t, conn)
	}
	assert.Equal(t, stream.ReasonDeleted, f.Reason)
	h.QuiesceReactors()
	conn.client.Sub(t, conn.scope, 0)
	refused := nextFeedFrame(t, conn)
	assert.Equal(t, stream.ReasonRefused, refused.Reason, "an erased chat has no feed")
	require.NotNil(t, refused.Problem)
	assert.Equal(t, problem.NotFound, refused.Problem.Code)
}

func TestChatFeed_ToolPayloadIsReadableByItsRef(t *testing.T) {
	h := newHarness(t)
	_, chatID, post := newFeedChat(t, h, "inputstub", inputStubProviderDescriptorYAML)
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"list"}`)
	conn := dialFeed(t, h, chatID, 0)
	untilReady(t, conn)

	post("tool_pre", `{"session_id":"sess-1","tool_use_id":"tool-1","tool_name":"Bash","tool_input":{"command":"ls -la"}}`)
	var call struct {
		InputRef string `json:"inputRef"`
	}
	for call.InputRef == "" {
		f := nextFeedFrame(t, conn)
		if f.T == stream.FrameRow && f.Row.Kind == ledger.KindToolCall {
			require.NoError(t, json.Unmarshal(f.Row.Body, &call))
			require.NotEmpty(t, call.InputRef, "the tool_call row names its input: %s", f.Row.Body)
		}
	}

	resp := h.raw(http.MethodGet, "/v0/chats/"+chatID+"/blobs/"+url.PathEscape(call.InputRef), nil, http.StatusOK)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.JSONEq(t, `{"command":"ls -la"}`, string(body))
	bad := h.raw(http.MethodGet, "/v0/chats/"+chatID+"/blobs/not-a-ref", nil, http.StatusBadRequest)
	require.NoError(t, bad.Body.Close())
}

var updateRowsFixture = flag.Bool("update-rows-fixture", false,
	"write the recorded subagent session's rows fixture for the web tests")

const recordedRowsFixture = "../../web/src/__tests__/__fixtures__/claude-subagent-session.rows.json"

// runIDs is what the daemon mints per run (chat, turn, runner, session ids)
// and the blob digests, some of whose content names the run's session.
var runIDs = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|[0-9a-f]{64}`)

// stableRows is the chat scope's replay with what differs between runs replaced,
// in order of appearance, by fixed placeholders; ids the recording carries stay.
// It reads the socket raw so each row keeps the bytes the daemon wrote.
func stableRows(t *testing.T, fc *feedConn, recorded string) []byte {
	t.Helper()
	var rows []json.RawMessage
	for {
		require.NoError(t, fc.client.conn.SetReadDeadline(time.Now().Add(45*time.Second)))
		_, raw, err := fc.client.conn.ReadMessage()
		require.NoError(t, err)
		var f struct {
			T     string          `json:"t"`
			Scope string          `json:"scope"`
			Row   json.RawMessage `json:"row"`
		}
		require.NoError(t, json.Unmarshal(raw, &f))
		if f.Scope != fc.scope {
			continue
		}
		if f.T == stream.FrameSynced {
			break
		}
		require.Equal(t, stream.FrameRow, f.T, "only rows come before synced")
		rows = append(rows, f.Row)
	}
	out, err := json.MarshalIndent(rows, "", "  ")
	require.NoError(t, err)
	names := map[string]string{}
	text := runIDs.ReplaceAllStringFunc(string(out), func(id string) string {
		if strings.Contains(recorded, id) {
			return id
		}
		if _, ok := names[id]; !ok && len(id) == 64 {
			names[id] = fmt.Sprintf("%064x", len(names)+1)
		} else if !ok {
			names[id] = fmt.Sprintf("id-%d", len(names)+1)
		}
		return names[id]
	})
	at := regexp.MustCompile(`"at": "[^"]+"`)
	n := 0
	text = at.ReplaceAllStringFunc(text, func(string) string {
		n++
		return fmt.Sprintf(`"at": "2026-10-01T12:%02d:%02dZ"`, n/60, n%60)
	})
	return append([]byte(text), '\n')
}

// The web transcript's tests render this session's real rows; the file is
// written only from here, so it never drifts from the ingress.
func TestFeedRows_RecordedSubagentSessionFixture(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	c.replayRange(t, 0, 1000)
	var recorded bytes.Buffer
	entries, err := os.ReadDir(subagentSessionDir)
	require.NoError(t, err)
	for _, e := range entries {
		raw, err := os.ReadFile(subagentSessionDir + "/" + e.Name())
		require.NoError(t, err)
		recorded.Write(raw)
	}

	got := stableRows(t, dialFeed(t, c.h, c.chatID, 0), recorded.String())

	if *updateRowsFixture {
		require.NoError(t, os.WriteFile(recordedRowsFixture, got, 0o600))
		return
	}
	want, err := os.ReadFile(recordedRowsFixture)
	require.NoError(t, err, "write it with -args -update-rows-fixture")
	assert.Equal(t, string(want), string(got), "the rows drifted: rerun with -args -update-rows-fixture")
}
