package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

type fakeWSReader struct{ chatsDir, worktree string }

func (f fakeWSReader) WorktreeDir(context.Context, string) (string, string, string, string, error) {
	return "", "", "", f.worktree, nil
}

func (f fakeWSReader) AgentChatsDir(context.Context, string) (string, error) {
	return f.chatsDir, nil
}

var _ seam.WorkspaceReader = fakeWSReader{}

func TestRewritePromptTextForDispatch_RewritesAnAttachmentReference(t *testing.T) {
	home := t.TempDir()
	chatsDir := filepath.Join(home, "chats")
	durableDir := worktreepath.AttachmentsDir(chatsDir, "chat-1")
	fileName, _, err := repoattachments.Store(durableDir, "ab12", "photo.png", []byte("bytes"))
	require.NoError(t, err)

	rs := &Runners{ws: fakeWSReader{chatsDir: chatsDir}}
	text := "![photo](chats/chat-1/attachments/" + fileName + ")"
	out, err := rs.rewritePromptTextForDispatch(context.Background(), "ws-1", "chat-1", text)
	require.NoError(t, err)

	assert.Contains(t, out, filepath.ToSlash(filepath.Join(durableDir, fileName)))
	assert.NotContains(t, out, "]("+"chats/chat-1/attachments/"+fileName+")",
		"the logical markdown link must be gone, replaced by the absolute path")
}

func TestRewritePromptTextForDispatch_NoAttachmentReference_ReturnsTextUnchanged(t *testing.T) {
	rs := &Runners{ws: fakeWSReader{chatsDir: filepath.Join(t.TempDir(), "chats")}}
	out, err := rs.rewritePromptTextForDispatch(context.Background(), "ws-1", "chat-1", "hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", out)
}

// boomWorkspaceReader fails AgentChatsDir every time, so rewritePromptTextForDispatch's
// own error branch is reachable independent of anything materializeAttachmentsForDispatch
// itself might fail on.
type boomWorkspaceReader struct{ seam.WorkspaceReader }

func (boomWorkspaceReader) AgentChatsDir(context.Context, string) (string, error) {
	return "", assertError("boom: chats dir lookup failed")
}

type assertError string

func (e assertError) Error() string { return string(e) }

func TestRewritePromptTextForDispatch_ChatsDirLookupFailure_IsWrapped(t *testing.T) {
	rs := &Runners{ws: boomWorkspaceReader{}}
	_, err := rs.rewritePromptTextForDispatch(context.Background(), "ws-1", "chat-1", "hello")
	require.Error(t, err)
	assert.ErrorContains(t, err, "chats dir")
	assert.ErrorContains(t, err, "boom")
}

// stubRunnerStoreForAPIPush answers only Get — the one call commitPromptSpawn
// makes once pushPromptOverAPI has succeeded. The embedded nil interface
// panics on anything else.
type stubRunnerStoreForAPIPush struct {
	agentrunner.EventStore
	terminalSession string
}

func (s stubRunnerStoreForAPIPush) Get(context.Context, string) (engineagents.Runner, error) {
	return engineagents.Runner{TerminalSession: s.terminalSession}, nil
}

// apiPushAttachmentTestDescriptor mirrors apiTransportTestDescriptor's shape
// (apiconn_internal_test.go) plus a prompt: event whose fresh step is a bare
// {text} passthrough — the minimal shape that exercises pushPromptOverAPI's
// real Dispatch("prompt", ...) call over a real (fake-server-backed)
// *engineagents.APIConn, so this test proves the REWRITTEN text is what
// actually reaches the wire, not just what materializeAttachmentsForDispatch
// returns in isolation.
const apiPushAttachmentTestDescriptor = `
id: api-push-test
spawn:
  cmd: acme
  interactive_required: true
events:
  session_start:
    in: thread/started
    map: { session_id: thread.id }
  turn_stop:
    in: turn/completed
    map:
      session_id: threadId
      message: "turn.items[type=agentMessage].text"
  prompt:
    fresh:
      - call: turn/start
        send:
          text: "{text}"
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve: [acme, serve]
    handshake: { call: initialize }
`

func apiPushAttachmentTestAgent(t *testing.T) engineagents.Agent {
	t.Helper()
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(home, "descriptors", "api-push-test.yaml"), []byte(apiPushAttachmentTestDescriptor), 0o600))
	a, err := engineagents.New().Get(context.Background(), home, "api-push-test")
	require.NoError(t, err)
	return a
}

// TestSubmitPromptOverAPI_MaterializesAnAttachmentBeforePushing is Task 8's
// end-to-end pin: a chat with a live api connection and an attachment
// reference in its prompt text gets the SAME absolute-path rewrite the
// spawnRunner/restart_tui path already gets, and the CLI on the wire
// receives the file's real absolute path, never the durable logical
// chats/<id>/attachments reference.
func TestSubmitPromptOverAPI_MaterializesAnAttachmentBeforePushing(t *testing.T) {
	home := t.TempDir()
	worktree := filepath.Join(home, "worktree")
	chatsDir := filepath.Join(home, "chats")
	durableDir := worktreepath.AttachmentsDir(chatsDir, "chat-1")
	fileName, _, err := repoattachments.Store(durableDir, "ab12", "photo.png", []byte("bytes"))
	require.NoError(t, err)

	received := make(chan string, 1)
	sockPath := fakeWSServer(t, func(conn *websocket.Conn) {
		_, msg, err := conn.ReadMessage() // turn/start
		require.NoError(t, err)
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		require.NoError(t, json.Unmarshal(msg, &req))
		var params struct {
			Text string `json:"text"`
		}
		require.NoError(t, json.Unmarshal(req.Params, &params))
		received <- params.Text
		resp, _ := json.Marshal(map[string]any{"id": req.ID, "result": map[string]any{}})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, resp))
		_, _, _ = conn.ReadMessage() // block until the client closes
	})

	agent := apiPushAttachmentTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, sockPath)
	require.NoError(t, err)
	defer apiConn.Close()

	rs := &Runners{
		apiConns:    newAPIConnRegistry(),
		ws:          fakeWSReader{chatsDir: chatsDir, worktree: worktree},
		prompts:     agentjournal.NewPromptRequests(),
		runnerStore: stubRunnerStoreForAPIPush{terminalSession: "term-1"},
	}
	rs.apiConns.set("runner-1", &apiconn{driver: apiConn, ctx: ctx})

	chat := domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}
	live := engineagents.Runner{ID: "runner-1", ProviderID: "api-push-test"}
	text := "![photo](chats/chat-1/attachments/" + fileName + ")"
	textHash := agentjournal.PromptTextHash(text)
	journalDir := rs.prompts.Dir(chatsDir, "chat-1")

	submission, handled, err := rs.submitPromptOverAPI(
		ctx, chat, journalDir, uuid.NewString(), textHash, live, worktree, text,
	)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, "runner-1", submission.RunnerID)
	assert.Equal(t, "term-1", submission.TerminalSessionID)

	var dispatched string
	select {
	case dispatched = <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the prompt dispatch to reach the wire")
	}
	assert.Contains(t, dispatched, filepath.ToSlash(filepath.Join(durableDir, fileName)),
		"the CLI must receive the file's real absolute path, not the logical reference")
	assert.NotContains(t, dispatched, "]("+"chats/chat-1/attachments/"+fileName+")")
}

// TestSubmitPromptOverAPI_PushFailureAfterSuccessfulMaterialize_StillMarksUncertain
// proves the reordering this task introduces (materialize, THEN push) does not
// disturb pushPromptOverAPI's own pre-existing failure handling: a successfully
// materialized (and rewritten) attachment reference still reaches the wire, and
// a subsequent wire-level failure is still marked uncertain exactly as it was
// before this task touched the function.
func TestSubmitPromptOverAPI_PushFailureAfterSuccessfulMaterialize_StillMarksUncertain(t *testing.T) {
	home := t.TempDir()
	worktree := filepath.Join(home, "worktree")
	chatsDir := filepath.Join(home, "chats")
	durableDir := worktreepath.AttachmentsDir(chatsDir, "chat-1")
	fileName, _, err := repoattachments.Store(durableDir, "ab12", "photo.png", []byte("bytes"))
	require.NoError(t, err)

	received := make(chan string, 1)
	sockPath := fakeWSServer(t, func(conn *websocket.Conn) {
		_, msg, err := conn.ReadMessage() // turn/start
		require.NoError(t, err)
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		require.NoError(t, json.Unmarshal(msg, &req))
		var params struct {
			Text string `json:"text"`
		}
		require.NoError(t, json.Unmarshal(req.Params, &params))
		received <- params.Text
		resp, _ := json.Marshal(map[string]any{
			"id":    req.ID,
			"error": map[string]any{"code": 1, "message": "boom: the CLI refused turn/start"},
		})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, resp))
		_, _, _ = conn.ReadMessage() // block until the client closes
	})

	agent := apiPushAttachmentTestAgent(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	apiConn, err := agent.StartAPIConn(ctx, sockPath)
	require.NoError(t, err)
	defer apiConn.Close()

	rs := &Runners{
		apiConns: newAPIConnRegistry(),
		ws:       fakeWSReader{chatsDir: chatsDir, worktree: worktree},
		prompts:  agentjournal.NewPromptRequests(),
	}
	rs.apiConns.set("runner-1", &apiconn{driver: apiConn, ctx: ctx})

	chat := domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}
	live := engineagents.Runner{ID: "runner-1", ProviderID: "api-push-test"}
	text := "![photo](chats/chat-1/attachments/" + fileName + ")"
	textHash := agentjournal.PromptTextHash(text)
	journalDir := rs.prompts.Dir(chatsDir, "chat-1")
	requestID := uuid.NewString()

	submission, handled, err := rs.submitPromptOverAPI(
		ctx, chat, journalDir, requestID, textHash, live, worktree, text,
	)
	assert.True(t, handled)
	assert.Equal(t, domain.AgentPromptSubmission{}, submission)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPromptOutcomeUnknown)

	var dispatched string
	select {
	case dispatched = <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the prompt dispatch to reach the wire")
	}
	assert.Contains(t, dispatched, filepath.ToSlash(filepath.Join(durableDir, fileName)),
		"materialization must still have run before the wire-level failure")

	record, found, lookupErr := rs.prompts.Lookup(journalDir, requestID, textHash)
	require.NoError(t, lookupErr)
	require.True(t, found)
	assert.Equal(t, agentjournal.PromptStateUncertain, record.State)
}

// TestSubmitPromptOverAPI_MaterializeFailure_MarksTheDispatchUncertain pins
// this path's own failure handling: a materialize-attachments failure before
// anything reaches the wire is treated exactly like pushPromptOverAPI's own
// failure branch just below it in submitPromptOverAPI (mark the durable
// dispatch record uncertain, return ErrPromptOutcomeUnknown) rather than
// leaving the record BEGIN wrote stuck in "dispatching" — every other failure
// branch after Begin succeeds in this function follows that same idiom.
func TestSubmitPromptOverAPI_MaterializeFailure_MarksTheDispatchUncertain(t *testing.T) {
	rs := &Runners{
		apiConns: newAPIConnRegistry(),
		ws:       boomWorkspaceReader{},
		prompts:  agentjournal.NewPromptRequests(),
	}
	// A dummy entry is enough: materialization fails before pushPromptOverAPI
	// ever dereferences the connection.
	rs.apiConns.set("runner-1", &apiconn{})

	journalDir := filepath.Join(t.TempDir(), "prompt-requests")
	chat := domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}
	live := engineagents.Runner{ID: "runner-1", ProviderID: "boom-provider"}
	text := "hello"
	textHash := agentjournal.PromptTextHash(text)
	requestID := uuid.NewString()

	submission, handled, err := rs.submitPromptOverAPI(
		context.Background(), chat, journalDir, requestID, textHash, live, "/worktree", text,
	)

	assert.True(t, handled)
	assert.Equal(t, domain.AgentPromptSubmission{}, submission)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPromptOutcomeUnknown)

	record, found, lookupErr := rs.prompts.Lookup(journalDir, requestID, textHash)
	require.NoError(t, lookupErr)
	require.True(t, found)
	assert.Equal(t, agentjournal.PromptStateUncertain, record.State)
}
