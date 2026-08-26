package apidriver_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/apidriver"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func loadCodexAPIDescriptor(t *testing.T) *spec.Descriptor {
	t.Helper()
	raw, err := os.ReadFile("../descriptor/descriptors-v3/codex.yaml")
	require.NoError(t, err)
	d, err := descriptor.ParseV3(raw)
	require.NoError(t, err)
	return d
}

// fakeCodexServer replays a scripted sequence of frames after completing the
// initialize handshake exactly as the real app-server does — see wsrpc's own
// tests for the base handshake plumbing; this adds the initialized notification
// step apidriver.Start performs before handing back a Driver.
func fakeCodexServer(t *testing.T, afterInit func(*websocket.Conn)) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "apidriver")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sockPath := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", sockPath)
	require.NoError(t, err)

	upgrader := websocket.Upgrader{}
	srv := &httptest.Server{
		Listener: ln,
		Config: &http.Server{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := upgrader.Upgrade(w, r, nil)
				require.NoError(t, err)
				defer conn.Close()

				_, msg, err := conn.ReadMessage() // initialize
				require.NoError(t, err)
				var req struct {
					ID json.RawMessage `json:"id"`
				}
				require.NoError(t, json.Unmarshal(msg, &req))
				resp, _ := json.Marshal(map[string]any{"id": req.ID, "result": map[string]string{}})
				require.NoError(t, conn.WriteMessage(websocket.TextMessage, resp))

				_, _, err = conn.ReadMessage() // initialized notification, discarded
				require.NoError(t, err)

				afterInit(conn)
			}),
		},
	}
	srv.Start()
	t.Cleanup(func() {
		srv.Close()
		_ = os.Remove(sockPath)
	})
	return sockPath
}

func TestStart_HandshakeThenDeliversCanonicalEvents(t *testing.T) {
	turnCompleted, err := os.ReadFile("../../testdata/fixtures/codex/turn_completed.json")
	require.NoError(t, err)

	sockPath := fakeCodexServer(t, func(conn *websocket.Conn) {
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, turnCompleted))
		_, _, _ = conn.ReadMessage() // block until the client closes
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath)
	require.NoError(t, err)
	defer drv.Close()

	select {
	case ev := <-drv.Events():
		require.Equal(t, "turn_stop", ev.Canonical)
		require.Nil(t, ev.AskID)
		require.Contains(t, string(ev.Raw), "threadId")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the dispatched event")
	}
}

func TestStart_AsksCarryAReplyChannel(t *testing.T) {
	sockPath := fakeCodexServer(t, func(conn *websocket.Conn) {
		ask, _ := json.Marshal(map[string]any{
			"id": 7, "method": "item/permissions/requestApproval",
			"params": map[string]string{"tool": "shell"},
		})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, ask))
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		var reply struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
		}
		require.NoError(t, json.Unmarshal(msg, &reply))
		require.Equal(t, 7, reply.ID)
		require.JSONEq(t, `{"decision":"approved"}`, string(reply.Result))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath)
	require.NoError(t, err)
	defer drv.Close()

	ev := <-drv.Events()
	require.Equal(t, "permission", ev.Canonical)
	require.NotNil(t, ev.AskID)
	require.NoError(t, drv.Reply(ev.AskID, []byte(`{"decision":"approved"}`)))
}

func TestStart_MalformedParamsAreDroppedNotFatal(t *testing.T) {
	turnCompleted, err := os.ReadFile("../../testdata/fixtures/codex/turn_completed.json")
	require.NoError(t, err)

	sockPath := fakeCodexServer(t, func(conn *websocket.Conn) {
		// A frame whose params is not a JSON object at all — must be skipped,
		// not crash the translate loop, and the NEXT valid frame must still land.
		bad, _ := json.Marshal(map[string]any{"method": "turn/completed", "params": "not-an-object"})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, bad))
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, turnCompleted))
		_, _, _ = conn.ReadMessage()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath)
	require.NoError(t, err)
	defer drv.Close()

	select {
	case ev := <-drv.Events():
		require.Equal(t, "turn_stop", ev.Canonical)
	case <-time.After(5 * time.Second):
		t.Fatal("the valid frame after the malformed one never arrived")
	}
}

// scriptedCall answers one JSON-RPC request by method name and returns the raw
// params it was called with, so a test can assert on the ACTUAL wire payload a
// Fresh/Resume/Action step sent — not just that some call happened.
type scriptedCall struct {
	method string
	result string // raw JSON to return as this call's "result"
}

// scriptedServer replays call/response pairs in order, verifying each
// incoming request's method matches the script and recording its raw params
// for the test to inspect afterward.
func scriptedServer(t *testing.T, script []scriptedCall) (sockPath string, seenParams *[]string) {
	t.Helper()
	seen := make([]string, 0, len(script))
	sockPath = fakeCodexServer(t, func(conn *websocket.Conn) {
		for _, step := range script {
			_, msg, err := conn.ReadMessage()
			require.NoError(t, err)
			var req struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			require.NoError(t, json.Unmarshal(msg, &req))
			require.Equal(t, step.method, req.Method, "unexpected call")
			seen = append(seen, string(req.Params))
			resp, _ := json.Marshal(map[string]any{
				"id":     req.ID,
				"result": json.RawMessage(step.result),
			})
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, resp))
		}
		_, _, _ = conn.ReadMessage() // block until the client closes
	})
	return sockPath, &seen
}

func TestEstablishSession_NoKnownSessionRunsFreshAndCapturesTheNewID(t *testing.T) {
	sockPath, seen := scriptedServer(t, []scriptedCall{
		{method: "thread/start", result: `{"thread":{"id":"t-123"}}`},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath)
	require.NoError(t, err)
	defer drv.Close()

	out, err := drv.EstablishSession(ctx, "prompt", map[string]string{"session_id": "", "cwd": "/work"})
	require.NoError(t, err)
	require.Equal(t, "t-123", out["session_id"])
	require.Contains(t, (*seen)[0], `"cwd":"/work"`)
	require.Contains(t, (*seen)[0], `"sandbox":"workspace-write"`)
}

// TestRegression_EstablishSessionFreshCarriesTheHandoffAsDeveloperInstructions
// pins the live bug: switching a chat to codex spawned it with the whole prior
// conversation correctly assembled into tctx.Context (spawnRunner already did
// that work, transport-agnostic), but api-transport's EstablishSession call
// dropped it on the floor — thread/start went out with only cwd/sandbox, so a
// codex chat switched-to from another provider answered its first message
// with zero memory of what came before. Verified against the real codex.yaml,
// not a test fixture, so a future edit that renames or removes the
// developerInstructions field breaks this test rather than shipping silently.
func TestRegression_EstablishSessionFreshCarriesTheHandoffAsDeveloperInstructions(t *testing.T) {
	sockPath, seen := scriptedServer(t, []scriptedCall{
		{method: "thread/start", result: `{"thread":{"id":"t-123"}}`},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath)
	require.NoError(t, err)
	defer drv.Close()

	_, err = drv.EstablishSession(ctx, "prompt", map[string]string{
		"session_id": "", "cwd": "/work", "context": "Claude said: I like turtles",
	})
	require.NoError(t, err)
	require.Contains(t, (*seen)[0], `"developerInstructions":"Claude said: I like turtles"`,
		"a virgin codex thread must open with the handed-off conversation, not silence")
}

func TestEstablishSession_KnownSessionRunsResumeNotFresh(t *testing.T) {
	sockPath, seen := scriptedServer(t, []scriptedCall{
		{method: "thread/resume", result: `{"thread":{"id":"known-1"},"cwd":"/work","model":"m","modelProvider":"p","sandbox":{"type":"workspaceWrite"},"approvalPolicy":"on-request","approvalsReviewer":"user"}`},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath)
	require.NoError(t, err)
	defer drv.Close()

	out, err := drv.EstablishSession(ctx, "prompt", map[string]string{"session_id": "known-1", "cwd": "/work"})
	require.NoError(t, err)
	require.Equal(t, "known-1", out["session_id"], "resume keeps the id the caller already had")
	require.Contains(t, (*seen)[0], `"threadId":"known-1"`)
}

func TestEstablishSession_SecondCallOnAnEstablishedConnectionIsANoop(t *testing.T) {
	sockPath, seen := scriptedServer(t, []scriptedCall{
		{method: "thread/start", result: `{"thread":{"id":"t-1"}}`},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath)
	require.NoError(t, err)
	defer drv.Close()

	_, err = drv.EstablishSession(ctx, "prompt", map[string]string{"session_id": "", "cwd": "/work"})
	require.NoError(t, err)

	// A second call — as a later message on the SAME connection would make —
	// must not call thread/start (or anything else) again: the script only
	// declared one call, and scriptedServer's ReadMessage would block forever
	// (failing the test's own deadline) if a second request arrived.
	out, err := drv.EstablishSession(ctx, "prompt", map[string]string{"session_id": "t-1", "cwd": "/work"})
	require.NoError(t, err)
	require.Equal(t, "t-1", out["session_id"])
	require.Len(t, *seen, 1, "only the first EstablishSession may reach the wire")
}

// TestRegression_EstablishSessionAlreadyEstablished_BlankCallerSessionIDFallsBackToRemembered
//
// pushPromptOverAPI's caller reads session_id off the RUNNER ROW, and that row is
// never rebound by a pure api-transport resume (thread/resume fires no
// thread/started notification for pumpAPIConn to carry into HandleSessionStart) —
// so the very first prompt sent after a switch BACK to an already-resumed codex
// passed session_id="" straight through. Dispatch → EstablishSession's established
// short-circuit used to hand that blank back unchanged, so turn/start received
// session_id="" and codex refused it outright ("invalid thread id: ... found 0"),
// permanently uncertain-ing every later prompt to that runner too. Confirmed live.
func TestRegression_EstablishSessionAlreadyEstablished_BlankCallerSessionIDFallsBackToRemembered(t *testing.T) {
	sockPath, _ := scriptedServer(t, []scriptedCall{
		{method: "thread/resume", result: `{"thread":{"id":"t-resumed"}}`},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath)
	require.NoError(t, err)
	defer drv.Close()

	_, err = drv.EstablishSession(ctx, "prompt", map[string]string{"session_id": "t-resumed", "cwd": "/work"})
	require.NoError(t, err)

	// The runner row's own CurrentSession, exactly as a switch-back hands it in.
	out, err := drv.EstablishSession(ctx, "prompt", map[string]string{"session_id": "", "cwd": "/work"})
	require.NoError(t, err)
	require.Equal(t, "t-resumed", out["session_id"],
		"a blank caller-supplied session_id must fall back to what THIS connection actually resumed, not overwrite it")
}

func TestDispatch_SendsTheStructuredActionPayloadAfterEstablishing(t *testing.T) {
	sockPath, seen := scriptedServer(t, []scriptedCall{
		{method: "thread/start", result: `{"thread":{"id":"t-9"}}`},
		{method: "turn/start", result: `{"turn":{"id":"turn-1","status":"inProgress"}}`},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath)
	require.NoError(t, err)
	defer drv.Close()

	out, err := drv.Dispatch(ctx, "prompt", map[string]string{
		"session_id": "", "cwd": "/work", "text": "hello there",
	})
	require.NoError(t, err)
	require.Equal(t, "t-9", out["session_id"])

	var turnStartParams struct {
		ThreadID string `json:"threadId"`
		Input    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"input"`
	}
	require.NoError(t, json.Unmarshal([]byte((*seen)[1]), &turnStartParams))
	require.Equal(t, "t-9", turnStartParams.ThreadID)
	require.Len(t, turnStartParams.Input, 1)
	require.Equal(t, "text", turnStartParams.Input[0].Type)
	require.Equal(t, "hello there", turnStartParams.Input[0].Text)
}

func TestDispatch_ASecondMessageOnAnEstablishedConnectionSkipsStraightToAction(t *testing.T) {
	sockPath, seen := scriptedServer(t, []scriptedCall{
		{method: "thread/start", result: `{"thread":{"id":"t-9"}}`},
		{method: "turn/start", result: `{"turn":{"id":"turn-1"}}`},
		{method: "turn/start", result: `{"turn":{"id":"turn-2"}}`},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath)
	require.NoError(t, err)
	defer drv.Close()

	_, err = drv.Dispatch(ctx, "prompt", map[string]string{"session_id": "", "cwd": "/work", "text": "first"})
	require.NoError(t, err)
	out, err := drv.Dispatch(ctx, "prompt", map[string]string{"session_id": "t-9", "cwd": "/work", "text": "second"})
	require.NoError(t, err)
	require.Equal(t, "t-9", out["session_id"])
	require.Len(t, *seen, 3, "the second message must not re-run thread/start")
}

func TestStart_MissingHandshakeCallIsAnError(t *testing.T) {
	raw := []byte(`
id: no-handshake
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve: [x]
  spawn:
    cmd: x
events:
  session_start:
    in: thread/started
    map: { session_id: thread.id }
`)
	d, err := descriptor.ParseV3(raw)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = apidriver.Start(ctx, d, "/nonexistent.sock")
	require.Error(t, err)
}

// TestSend_MergesValuesRememberedFromEarlierCaptures reproduces the codex
// interrupt shape live-confirmed against codex-cli 0.149.1: turn/interrupt
// needs BOTH threadId (from thread/start, captured at EstablishSession) and
// turnId (from turn/start, captured at Dispatch) — neither of which the caller
// supplies to Send itself. Send must source both from what this connection has
// already remembered.
func TestSend_MergesValuesRememberedFromEarlierCaptures(t *testing.T) {
	sockPath, seen := scriptedServer(t, []scriptedCall{
		{method: "thread/start", result: `{"thread":{"id":"t-9"}}`},
		{method: "turn/start", result: `{"turn":{"id":"turn-1"}}`},
		{method: "turn/interrupt", result: `{}`},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath)
	require.NoError(t, err)
	defer drv.Close()

	_, err = drv.Dispatch(ctx, "prompt", map[string]string{"session_id": "", "cwd": "/work", "text": "hi"})
	require.NoError(t, err)

	require.NoError(t, drv.Send(ctx, "interrupt", nil))

	require.Contains(t, (*seen)[2], `"threadId":"t-9"`)
	require.Contains(t, (*seen)[2], `"turnId":"turn-1"`)
}

// TestSend_PropagatesAJSONRPCErrorFromTheReply pins that Send waits for a real
// reply rather than firing a notification — live-confirmed that codex's own
// turn/interrupt is request/response: a malformed call comes back a JSON-RPC
// error, not silence, and Send has to surface that rather than reporting
// success on a call the server actually rejected.
func TestSend_PropagatesAJSONRPCErrorFromTheReply(t *testing.T) {
	sockPath := fakeCodexServer(t, func(conn *websocket.Conn) {
		_, msg, err := conn.ReadMessage() // thread/start
		require.NoError(t, err)
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		require.NoError(t, json.Unmarshal(msg, &req))
		resp, _ := json.Marshal(map[string]any{"id": req.ID, "result": map[string]any{"thread": map[string]string{"id": "t-9"}}})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, resp))

		_, msg, err = conn.ReadMessage() // interrupt
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(msg, &req))
		errResp, _ := json.Marshal(map[string]any{
			"id":    req.ID,
			"error": map[string]any{"code": -32600, "message": "missing field `turnId`"},
		})
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, errResp))
		_, _, _ = conn.ReadMessage() // block until the client closes
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath)
	require.NoError(t, err)
	defer drv.Close()

	_, err = drv.EstablishSession(ctx, "prompt", map[string]string{"session_id": "", "cwd": "/work"})
	require.NoError(t, err)

	err = drv.Send(ctx, "interrupt", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing field")
}

// TestInjectAt_RunsTheDeclaredContextStepAfterResume is the fix for the gap left
// by apiOwnsResume (chat/internal/runner/prompts.go): a resumed codex thread has
// no CLI argv left to carry "what happened while you were away" (the redundant
// hooks-only PTY is deliberately starved of both the native resume id and any
// positional prompt — handing either to a second writer on the same thread is
// what corrupted the switch in the first place). codex.yaml's own inject: at:
// context step (thread/inject_items) is what reaches the resumed thread
// instead, called from apiconn.go's applyAPITransport right after
// EstablishSession's resume completes — this pins that the driver actually runs
// it, templating {session_id} from what THIS connection just resumed and
// {context} from the caller's document into the item shape confirmed LIVE
// against a real codex-cli 0.149.1 thread as the one that is actually recalled
// on the next turn — a "message"/role "user" Responses API item. codex.yaml's
// items field has no schema at all (ThreadInjectItemsParams declares its
// element type unconstrained), so a wrong role here fails silently — codex
// accepts and discards a "developer" or "system" item with no error and no
// later trace of it — which is exactly why this pins the exact shape rather
// than a loose substring.
func TestInjectAt_RunsTheDeclaredContextStepAfterResume(t *testing.T) {
	sockPath, seen := scriptedServer(t, []scriptedCall{
		{method: "thread/resume", result: `{"thread":{"id":"sid-1"}}`},
		{method: "thread/inject_items", result: `{}`},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath)
	require.NoError(t, err)
	defer drv.Close()

	_, err = drv.EstablishSession(ctx, "prompt", map[string]string{"session_id": "sid-1", "cwd": "/work"})
	require.NoError(t, err)

	err = drv.InjectAt(ctx, "context", map[string]string{
		"session_id": "sid-1",
		"context":    "while you were away, this happened",
	})
	require.NoError(t, err)

	require.Len(t, *seen, 2, "InjectAt must reach the wire as its own call, after resume")
	var params struct {
		ThreadID string `json:"threadId"`
		Items    []struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte((*seen)[1]), &params))
	require.Equal(t, "sid-1", params.ThreadID)
	require.Len(t, params.Items, 1)
	assert.Equal(t, "message", params.Items[0].Type)
	assert.Equal(t, "user", params.Items[0].Role, "role user is what a live probe showed codex actually recalling")
	require.Len(t, params.Items[0].Content, 1)
	assert.Equal(t, "input_text", params.Items[0].Content[0].Type)
	assert.Equal(t, "while you were away, this happened", params.Items[0].Content[0].Text)
}

// TestInjectAt_UndeclaredMomentIsANoop: a descriptor with nothing declared for
// at (codex declares only "mcp" and "context", never "resume") must not call
// anything — the same declarative-capability shape ContextSteps already has for
// a provider with no use for a given moment.
func TestInjectAt_UndeclaredMomentIsANoop(t *testing.T) {
	sockPath, seen := scriptedServer(t, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath)
	require.NoError(t, err)
	defer drv.Close()

	err = drv.InjectAt(ctx, "resume", map[string]string{"session_id": "sid-1"})
	require.NoError(t, err)
	require.Empty(t, *seen, "an undeclared moment must never reach the wire")
}
