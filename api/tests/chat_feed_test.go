//go:build integration

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
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

type feedFrame struct {
	Type string `json:"type"`
	Row  *struct {
		Seq  int64  `json:"seq"`
		Kind string `json:"kind"`
	} `json:"row"`
	Epoch string `json:"epoch"`
	Seq   int64  `json:"seq"`
	Delta *struct {
		ID      string `json:"id"`
		Stream  string `json:"stream"`
		N       int    `json:"n"`
		Text    string `json:"text"`
		Replace bool   `json:"replace"`
	} `json:"delta"`
}

func dialFeed(t *testing.T, h *harness, chatID string, since int64) *websocket.Conn {
	t.Helper()
	return h.dial("/v0/chats/" + chatID + "/feed?since=" + strconv.FormatInt(since, 10))
}

// nextFeedFrame reads one frame; a frame that never comes fails the test.
func nextFeedFrame(t *testing.T, conn *websocket.Conn) feedFrame {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(45*time.Second)))
	_, raw, err := conn.ReadMessage()
	require.NoError(t, err, "the feed sent no frame within 45s")
	var f feedFrame
	require.NoError(t, json.Unmarshal(raw, &f), "%s", raw)
	return f
}

// untilReady reads the replay and returns its row seqs and the ready frame.
func untilReady(t *testing.T, conn *websocket.Conn) ([]int64, feedFrame) {
	t.Helper()
	var seqs []int64
	for {
		f := nextFeedFrame(t, conn)
		if f.Type == "ready" {
			return seqs, f
		}
		require.Equal(t, "row", f.Type, "only rows come before ready")
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
	assert.NotEmpty(t, ready.Epoch)

	post("message_delta", delta("msg-one", 0, true, "HI"))
	live := nextFeedFrame(t, conn)
	require.Equal(t, "row", live.Type)
	assert.Equal(t, ready.Seq+1, live.Row.Seq)
	assert.Equal(t, "assistant_text", live.Row.Kind)
}

func TestChatFeed_ResumeSendsOnlyNewerRows(t *testing.T) {
	h := newHarness(t)
	_, chatID, post := newFeedChat(t, h, "streamstub", streamStubProviderDescriptorYAML)
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"hello"}`)
	_, head := untilReady(t, dialFeed(t, h, chatID, 0))
	require.GreaterOrEqual(t, head.Seq, int64(2))

	seqs, ready := untilReady(t, dialFeed(t, h, chatID, head.Seq-2))

	assert.Equal(t, []int64{head.Seq - 1, head.Seq}, seqs)
	assert.Equal(t, head.Seq, ready.Seq)
	assert.Equal(t, head.Epoch, ready.Epoch, "one daemon, one epoch")
}

// postHookAsync posts a hook as l's CLI from a goroutine, reporting what went wrong.
func postHookAsync(h *harness, l kit.Launch, base, provider, event, payload string) error {
	url := fmt.Sprintf("%s%s/chats/runners/%s/hooks/%s/%s", h.url, base, l.RunnerID, provider, event)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, strings.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("X-Crowbar-Runner-Token", l.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.server.Client().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: status %d", event, resp.StatusCode)
	}
	return nil
}

func TestChatFeed_RowsCommittedWhileAttachingAreNeitherLostNorDoubled(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "streamstub")
	postHook(t, h, repoBase(imported), "streamstub", runnerID, "session_start", `{"session_id":"sess-1"}`)
	postHook(t, h, repoBase(imported), "streamstub", runnerID, "user_prompt", `{"session_id":"sess-1","prompt":"go"}`)

	posted := make(chan error, 1)
	l := stubLaunch(t, h, "streamstub", runnerID)
	go func() {
		for i := range 30 {
			if err := postHookAsync(h, l, repoBase(imported), "streamstub", "message_delta",
				delta("msg-one", i, false, fmt.Sprintf("part %d ", i))); err != nil {
				posted <- err
				return
			}
		}
		posted <- nil
	}()
	conn := dialFeed(t, h, chatID, 0)
	require.NoError(t, <-posted)
	_, head := untilReady(t, dialFeed(t, h, chatID, 0))

	var seqs []int64
	for len(seqs) == 0 || seqs[len(seqs)-1] < head.Seq {
		if f := nextFeedFrame(t, conn); f.Type == "row" {
			seqs = append(seqs, f.Row.Seq)
		}
	}

	assert.Equal(t, contiguous(1, head.Seq), seqs, "each row once, in order, across the attach")
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
		if f.Type == "row" {
			assert.Equal(t, "tool_call", f.Row.Kind, "no row holds the deltas")
			assert.Equal(t, ready.Seq+1, f.Row.Seq)
			break
		}
		require.Equal(t, "delta", f.Type)
		assert.Equal(t, "answer", f.Delta.Stream)
		assert.False(t, f.Delta.Replace)
		deltas = append(deltas, fmt.Sprintf("%d:%s", f.Delta.N, f.Delta.Text))
	}
	assert.Equal(t, []string{"1:a", "2:b"}, deltas, "each frame is only its new text, numbered")

	again := dialFeed(t, h, chatID, 0)
	untilReady(t, again)
	snap := nextFeedFrame(t, again)
	require.Equal(t, "delta", snap.Type)
	assert.True(t, snap.Delta.Replace)
	assert.Equal(t, 2, snap.Delta.N)
	assert.Equal(t, "ab", snap.Delta.Text)
	post("tool_pre", `{"session_id":"sess-1","tool_use_id":"tool-2","tool_name":"Bash"}`)
	assert.Equal(t, "row", nextFeedFrame(t, again).Type, "the snapshot is one frame per live stream")
}

func TestChatFeed_EpochChangesAcrossARestart(t *testing.T) {
	home := t.TempDir()
	h1 := newHarnessAt(t, home)
	_, chatID, post := newFeedChat(t, h1, "streamstub", streamStubProviderDescriptorYAML)
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"hello"}`)
	_, before := untilReady(t, dialFeed(t, h1, chatID, 0))

	h1.shutdown()
	h2 := newHarnessAt(t, home)
	seqs, after := untilReady(t, dialFeed(t, h2, chatID, 0))

	assert.NotEqual(t, before.Epoch, after.Epoch, "a restart is a new epoch")
	assert.GreaterOrEqual(t, after.Seq, before.Seq, "seqs continue across it")
	assert.Equal(t, contiguous(1, after.Seq), seqs)
}

func TestChatFeed_DeletingTheChatEndsItsFeed(t *testing.T) {
	h := newHarness(t)
	imported, chatID, _ := newFeedChat(t, h, "streamstub", streamStubProviderDescriptorYAML)
	conn := dialFeed(t, h, chatID, 0)
	untilReady(t, conn)

	_ = h.raw(http.MethodDelete, repoBase(imported)+"/chats/"+chatID, nil, http.StatusAccepted).Body.Close()

	var last feedFrame
	var err error
	for err == nil {
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(45*time.Second)))
		var raw []byte
		if _, raw, err = conn.ReadMessage(); err == nil {
			require.NoError(t, json.Unmarshal(raw, &last))
		}
	}
	assert.Equal(t, "deleted", last.Type)
	var closed *websocket.CloseError
	require.ErrorAs(t, err, &closed)
	assert.Equal(t, 4001, closed.Code)
	h.QuiesceReactors()
	gone, resp, err := websocket.DefaultDialer.Dial("ws"+h.url[len("http"):]+"/v0/chats/"+chatID+"/feed", nil)
	require.Error(t, err)
	require.Nil(t, gone)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "an erased chat has no feed")
}
