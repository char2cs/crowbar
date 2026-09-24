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
	drv, err := apidriver.Start(ctx, d, sockPath, nil)
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
			"id": 7, "method": "item/commandExecution/requestApproval",
			"params": map[string]string{"command": "curl https://example.com"},
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
		require.JSONEq(t, `{"decision":"accept"}`, string(reply.Result))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath, nil)
	require.NoError(t, err)
	defer drv.Close()

	ev := <-drv.Events()
	require.Equal(t, "permission", ev.Canonical)
	require.NotNil(t, ev.AskID)
	require.NoError(t, drv.Reply(ev.AskID, []byte(`{"decision":"accept"}`)))
}

// TestStart_ByWireOverlayNamesTheToolOnThePermissionCard is F1 of the
// descriptor channel-split design, proven end to end over the real transport:
// codex's api-side permission payload carries no `tool`/`params` field on
// EITHER wire method (confirmed live, codex-cli 0.149.1 — see codex.yaml's
// own comment on permission.api), so before by_wire: existed the resulting
// card's tool name was always empty. by_wire: derives it from WHICH wire
// method actually matched — this drives both real wire names through the
// SAME live translateLoop production uses and checks the literal each one
// injects.
func TestStart_ByWireOverlayNamesTheToolOnThePermissionCard(t *testing.T) {
	for wire, want := range map[string]string{
		"item/commandExecution/requestApproval": "commandExecution",
		"item/fileChange/requestApproval":       "fileChange",
	} {
		t.Run(wire, func(t *testing.T) {
			sockPath := fakeCodexServer(t, func(conn *websocket.Conn) {
				ask, _ := json.Marshal(map[string]any{
					"id": 7, "method": wire,
					// Neither real payload carries `tool` or `params` — see
					// codex.yaml's own comment. reason is the one field both
					// real captures share.
					"params": map[string]string{"reason": "why"},
				})
				require.NoError(t, conn.WriteMessage(websocket.TextMessage, ask))
				_, _, _ = conn.ReadMessage() // block until the client replies/closes
			})

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			d := loadCodexAPIDescriptor(t)
			drv, err := apidriver.Start(ctx, d, sockPath, nil)
			require.NoError(t, err)
			defer drv.Close()

			ev := <-drv.Events()
			require.Equal(t, "permission", ev.Canonical)

			var params map[string]any
			require.NoError(t, json.Unmarshal(ev.Raw, &params))
			assert.Equal(t, want, params["tool"],
				"by_wire: must derive the tool identity from the matched wire method")
			require.NoError(t, drv.Reply(ev.AskID, []byte(`{"decision":"accept"}`)))
		})
	}
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
	drv, err := apidriver.Start(ctx, d, sockPath, nil)
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
	// errCode/errMessage answer with a JSON-RPC ERROR instead of a result.
	// Non-zero errCode selects that branch; result is ignored then.
	errCode    int
	errMessage string
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
			body := map[string]any{"id": req.ID, "result": json.RawMessage(step.result)}
			if step.errCode != 0 {
				body = map[string]any{"id": req.ID, "error": map[string]any{
					"code": step.errCode, "message": step.errMessage,
				}}
			}
			resp, _ := json.Marshal(body)
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
	drv, err := apidriver.Start(ctx, d, sockPath, nil)
	require.NoError(t, err)
	defer drv.Close()

	out, err := drv.EstablishSession(ctx, "prompt", map[string]string{
		"session_id": "", "cwd": "/work",
		"permission.sandbox": "workspace-write", "permission.approvalPolicy": "on-request",
	})
	require.NoError(t, err)
	require.Equal(t, "t-123", out["session_id"])
	require.Contains(t, (*seen)[0], `"cwd":"/work"`)
	require.Contains(t, (*seen)[0], `"sandbox":"workspace-write"`)
	require.Contains(t, (*seen)[0], `"approvalPolicy":"on-request"`)
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
	drv, err := apidriver.Start(ctx, d, sockPath, nil)
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
	drv, err := apidriver.Start(ctx, d, sockPath, nil)
	require.NoError(t, err)
	defer drv.Close()

	out, err := drv.EstablishSession(ctx, "prompt", map[string]string{"session_id": "known-1", "cwd": "/work"})
	require.NoError(t, err)
	require.Equal(t, "known-1", out["session_id"], "resume keeps the id the caller already had")
	require.Contains(t, (*seen)[0], `"threadId":"known-1"`)
}

// TestRegression_EstablishSessionResumeAlsoCarriesTheCurrentPermissionLevel
// pins the codex resume gap: thread/resume's own send: template used to
// carry only threadId/cwd, never {permission.sandbox}/{permission.
// approvalPolicy} — unlike thread/start's. So a chat whose permission level
// changed (an explicit SetChatPermissionLevel pick, or an inherited chat
// following a later global-default change — see selection.go) after its
// codex thread had already started kept resuming under the sandbox the
// thread was ORIGINALLY created with, forever: RestartRequired forces a
// restart on the level change, but a restart that only ever RESUMES never
// applied the new choice.
//
// Confirmed live and safe against the real codex-cli 0.154.0 app-server
// (throwaway CODEX_HOME/cwd, no repos or ~/.crowbar touched): thread/resume
// accepted sandbox/approvalPolicy in its params with no parse/validation
// error, failing only on the separate, already-documented "no rollout yet"
// condition (session_lost_codes) for a thread with no completed turn —
// exactly what codex.yaml's own comment on that code already says to
// expect. ThreadResumeParams's own published JSON schema (`codex app-server
// generate-json-schema`) also declares both fields, typed identically to
// ThreadStartParams's.
func TestRegression_EstablishSessionResumeAlsoCarriesTheCurrentPermissionLevel(t *testing.T) {
	sockPath, seen := scriptedServer(t, []scriptedCall{
		{method: "thread/resume", result: `{"thread":{"id":"known-1"},"cwd":"/work","model":"m","modelProvider":"p","sandbox":{"type":"dangerFullAccess"},"approvalPolicy":"never","approvalsReviewer":"user"}`},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath, nil)
	require.NoError(t, err)
	defer drv.Close()

	_, err = drv.EstablishSession(ctx, "prompt", map[string]string{
		"session_id": "known-1", "cwd": "/work",
		"permission.sandbox": "danger-full-access", "permission.approvalPolicy": "never",
	})
	require.NoError(t, err)
	require.Contains(t, (*seen)[0], `"sandbox":"danger-full-access"`,
		"a resumed thread must be handed the CURRENT permission level too, not just a fresh one")
	require.Contains(t, (*seen)[0], `"approvalPolicy":"never"`)
}

func TestEstablishSession_SecondCallOnAnEstablishedConnectionIsANoop(t *testing.T) {
	sockPath, seen := scriptedServer(t, []scriptedCall{
		{method: "thread/start", result: `{"thread":{"id":"t-1"}}`},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath, nil)
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
	drv, err := apidriver.Start(ctx, d, sockPath, nil)
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
	drv, err := apidriver.Start(ctx, d, sockPath, nil)
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
	drv, err := apidriver.Start(ctx, d, sockPath, nil)
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
	_, err = apidriver.Start(ctx, d, "/nonexistent.sock", nil)
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
	drv, err := apidriver.Start(ctx, d, sockPath, nil)
	require.NoError(t, err)
	defer drv.Close()

	_, err = drv.Dispatch(ctx, "prompt", map[string]string{"session_id": "", "cwd": "/work", "text": "hi"})
	require.NoError(t, err)

	require.NoError(t, drv.Send(ctx, "interrupt", nil))

	require.Contains(t, (*seen)[2], `"threadId":"t-9"`)
	require.Contains(t, (*seen)[2], `"turnId":"turn-1"`)
}

// TestRegression_InterruptRacingANewTurnStart_MustNotSendTheStalePreviousTurnID
// pins the "Codex stop doesn't work sometimes" bug. remembered["turn_id"] is
// only OVERWRITTEN once a turn/start call's reply lands and its capture: runs
// — nothing clears it the instant the NEXT turn/start is actually SENT. So a
// Stop that races in after a chat is already reported Working (which fires off
// the userMessage item/started notification arriving over this SAME
// connection, entirely independent of this call's own reply) but before THIS
// turn/start's reply has come back finds remembered still holding the
// PREVIOUS, already-finished turn's id — and Send happily ships turn/interrupt
// naming that dead turn. Confirmed live behaviour (codex.yaml's own interrupt:
// doc comment): turn/interrupt against an id that is not the CURRENTLY open
// turn is not what StopChat intends either way — an empty id fails loudly and
// falls back to a full stop (already handled), but a stale, well-formed one
// risks silently targeting a turn that already ended while the real one
// keeps generating, which is indistinguishable from Stop doing nothing at all.
func TestRegression_InterruptRacingANewTurnStart_MustNotSendTheStalePreviousTurnID(t *testing.T) {
	turn2StartSeen := make(chan struct{})
	interruptParams := make(chan string, 1)

	sockPath := fakeCodexServer(t, func(conn *websocket.Conn) {
		readReq := func() (id json.RawMessage, method, params string) {
			_, msg, err := conn.ReadMessage()
			require.NoError(t, err)
			var req struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			require.NoError(t, json.Unmarshal(msg, &req))
			return req.ID, req.Method, string(req.Params)
		}
		respond := func(id json.RawMessage, result string) {
			resp, _ := json.Marshal(map[string]any{"id": id, "result": json.RawMessage(result)})
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, resp))
		}

		id, method, _ := readReq() // thread/start
		require.Equal(t, "thread/start", method)
		respond(id, `{"thread":{"id":"t-9"}}`)

		id, method, _ = readReq() // turn/start #1 (turn-1)
		require.Equal(t, "turn/start", method)
		respond(id, `{"turn":{"id":"turn-1"}}`)

		// turn/start #2: read the request (this is what unblocks the test's own
		// Dispatch goroutine below), but deliberately withhold the reply — the
		// exact window a racing Stop would land in against a real, slower
		// codex app-server.
		id2, method, _ := readReq()
		require.Equal(t, "turn/start", method)
		close(turn2StartSeen)

		idI, methodI, paramsI := readReq() // turn/interrupt races in here
		require.Equal(t, "turn/interrupt", methodI)
		interruptParams <- paramsI
		respond(idI, `{}`)

		respond(id2, `{"turn":{"id":"turn-2"}}`)
		_, _, _ = conn.ReadMessage() // block until the client closes
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath, nil)
	require.NoError(t, err)
	defer drv.Close()

	// Turn 1 completes in full: remembered["turn_id"] == "turn-1".
	_, err = drv.Dispatch(ctx, "prompt", map[string]string{"session_id": "", "cwd": "/work", "text": "first"})
	require.NoError(t, err)

	// Turn 2 begins on a separate goroutine — its turn/start reaches the wire
	// but the fake server withholds the reply, mirroring a real turn that has
	// only just started.
	turn2Done := make(chan error, 1)
	go func() {
		_, err := drv.Dispatch(ctx, "prompt", map[string]string{"session_id": "t-9", "cwd": "/work", "text": "second"})
		turn2Done <- err
	}()

	select {
	case <-turn2StartSeen:
	case <-time.After(5 * time.Second):
		t.Fatal("turn/start for the second turn never reached the fake server")
	}

	// A Stop landing in exactly this window.
	require.NoError(t, drv.Send(ctx, "interrupt", nil))

	var params string
	select {
	case params = <-interruptParams:
	case <-time.After(5 * time.Second):
		t.Fatal("turn/interrupt never reached the fake server")
	}
	require.NotContains(t, params, `"turnId":"turn-1"`,
		"turn/interrupt raced against a new turn/start in flight must not name the PREVIOUS, already-finished turn")

	require.NoError(t, <-turn2Done)
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
	drv, err := apidriver.Start(ctx, d, sockPath, nil)
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
	drv, err := apidriver.Start(ctx, d, sockPath, nil)
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
	drv, err := apidriver.Start(ctx, d, sockPath, nil)
	require.NoError(t, err)
	defer drv.Close()

	err = drv.InjectAt(ctx, "resume", map[string]string{"session_id": "sid-1"})
	require.NoError(t, err)
	require.Empty(t, *seen, "an undeclared moment must never reach the wire")
}

// The two live-captured refusals (codex-cli 0.149.1) for a thread the server
// no longer has. Both carry the same code, which is the ONLY part of them Go
// is allowed to know — the wording is the provider's, and the code is what
// codex.yaml declares under runtime.api.session_lost_codes.
const (
	lostSessionCode      = -32600
	lostOnResumeMessage  = "no rollout found for thread id t-dead"
	lostOnTurnStartError = "thread not found: t-1"
)

// TestRegression_LostSessionOnResumeRebindsToAFreshThread pins the wedge that
// made a codex chat permanently unreachable. codex writes no rollout for a
// thread until a turn against it COMPLETES, so a thread that was started but
// never finished a turn cannot be resumed once its app-server is replaced —
// which happens on every restart. Resume then failed, the whole api connection
// was torn down, and because nothing ever cleared the dead id off the chat,
// every later spawn re-ran the identical doomed thread/resume. Observed four
// times against one thread id in a single evening's daemon log.
//
// The recovery is the Fresh path — the same thread/start a chat's first-ever
// message runs — because the old thread is genuinely gone provider-side and no
// retry of it can ever succeed.
func TestRegression_LostSessionOnResumeRebindsToAFreshThread(t *testing.T) {
	sockPath, seen := scriptedServer(t, []scriptedCall{
		{method: "thread/resume", errCode: lostSessionCode, errMessage: lostOnResumeMessage},
		{method: "thread/start", result: `{"thread":{"id":"t-new"}}`},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	origins := &recordedOrigins{}
	drv, err := apidriver.Start(ctx, d, sockPath, origins.claim)
	require.NoError(t, err)
	defer drv.Close()

	out, err := drv.EstablishSession(ctx, "prompt", map[string]string{
		"session_id": "t-dead", "cwd": "/work", "context": "what came before",
		"permission.sandbox": "workspace-write", "permission.approvalPolicy": "on-request",
	})
	require.NoError(t, err, "a session the provider has forgotten must be replaced, not fatal")
	require.Equal(t, "t-new", out["session_id"], "the caller must be handed the REPLACEMENT id")

	require.Len(t, *seen, 2, "resume must be tried first, then exactly one thread/start")
	require.Contains(t, (*seen)[1], `"sandbox":"workspace-write"`,
		"the replacement thread must be born with the same settings as the original")
	require.Contains(t, (*seen)[1], `"developerInstructions":"what came before"`,
		"the replacement thread must carry the handoff, or the chat silently loses its history")

	// The failed resume's OWN claim must be settled before establishFresh opens
	// its own: claims nest via originatedSessions' pending counter rather than
	// handing over, so one left open here never closes — and a permanently
	// pending claim makes the hook ingress read EVERY conversation the provider
	// announces on this connection, child threads included, as Crowbar's own.
	assert.Equal(t, []string{"t-new"}, origins.ids,
		"only the replacement actually came of this: the dead id must report nothing")
	assert.Equal(t, 2, origins.claims, "the failed establish and its fresh replacement each claim once")
	assert.Zero(t, origins.open, "every claim must be closed, the failed one included")
}

// recordedOrigins collects the sessions a connection reports having produced
// ITSELF, and asserts the claim was open before the id existed — the ordering
// the runner layer depends on, since a provider can announce a new session over
// this same connection before the call that creates it has returned.
type recordedOrigins struct {
	open   int
	claims int
	ids    []string
}

func (r *recordedOrigins) claim() func(string) {
	r.open++
	r.claims++
	return func(id string) {
		r.open--
		if id != "" {
			r.ids = append(r.ids, id)
		}
	}
}

// establishAtSpawn runs the spawn-time establish the recovery tests below all
// start from: it is what latches `established`, and the only call ever handed
// the sandbox/approval/context settings a later message push does not carry.
func establishAtSpawn(t *testing.T, ctx context.Context, drv *apidriver.Driver) {
	t.Helper()
	_, err := drv.EstablishSession(ctx, "prompt", map[string]string{
		"session_id": "", "cwd": "/work", "context": "what came before",
		"permission.sandbox": "workspace-write", "permission.approvalPolicy": "on-request",
	})
	require.NoError(t, err)
}

// TestRegression_LostSessionOnTurnStartReentersTheSameConversation pins what a
// provider actually means when it says it does not have a thread: an app-server
// pages an idle thread out of memory, which is precisely what thread/resume
// exists for. Measured against the real incident — the same app-server process
// that had created the thread nine hours earlier, still alive, reported it
// unknown on the next prompt.
//
// The recovery used to blank the id and run Fresh, which starts an EMPTY
// conversation: the user's own history was abandoned while the descriptor was
// still declaring, for this same event, a provider-sanctioned way back into it.
// Resume with the id we still hold comes FIRST.
func TestRegression_LostSessionOnTurnStartReentersTheSameConversation(t *testing.T) {
	sockPath, seen := scriptedServer(t, []scriptedCall{
		{method: "thread/start", result: `{"thread":{"id":"t-1"}}`},
		{method: "turn/start", errCode: lostSessionCode, errMessage: lostOnTurnStartError},
		{method: "thread/resume", result: `{"thread":{"id":"t-1"}}`},
		{method: "turn/start", result: `{"turn":{"id":"turn-9"}}`},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	origins := &recordedOrigins{}
	drv, err := apidriver.Start(ctx, d, sockPath, origins.claim)
	require.NoError(t, err)
	defer drv.Close()
	establishAtSpawn(t, ctx, drv)

	// A later message push: carries only the fields a prompt has, exactly as
	// pushPromptOverAPI sends them.
	out, err := drv.Dispatch(ctx, "prompt", map[string]string{
		"session_id": "t-1", "cwd": "/work", "text": "are you there?",
	})
	require.NoError(t, err, "a prompt refused for a paged-out thread must be re-delivered, not wedged")
	require.Equal(t, "t-1", out["session_id"],
		"the recovered session is the user's OWN conversation, not a replacement for it")

	require.Len(t, *seen, 4, "expected start, failed turn, resume, retried turn")
	require.Contains(t, (*seen)[2], `"threadId":"t-1"`,
		"recovery must re-enter the held conversation before it considers abandoning it")
	require.Contains(t, (*seen)[2], `"sandbox":"workspace-write"`,
		"recovery must reuse the settings the connection was born with, "+
			"not the bare field set a message push carries")
	require.Contains(t, (*seen)[3], `"threadId":"t-1"`, "the retry must name the SAME thread")
	require.Contains(t, (*seen)[3], `"are you there?"`, "the user's prompt must actually be delivered")

	assert.Equal(t, []string{"t-1", "t-1"}, origins.ids,
		"the spawn-time establish claims the thread it minted, and the recovery claims the "+
			"same conversation again — a recovered session is Crowbar's own even when its id did not change")
	assert.Zero(t, origins.open, "every claim must be closed")
}

// TestRegression_LostSessionFallsBackToFreshOnlyAfterResumeAlsoFails is the
// other half of the same rule. Fresh throws the conversation away, so it is the
// LAST answer, taken only once the provider has refused the held id on its own
// declared resume path too — a thread that genuinely no longer exists anywhere.
func TestRegression_LostSessionFallsBackToFreshOnlyAfterResumeAlsoFails(t *testing.T) {
	sockPath, seen := scriptedServer(t, []scriptedCall{
		{method: "thread/start", result: `{"thread":{"id":"t-1"}}`},
		{method: "turn/start", errCode: lostSessionCode, errMessage: lostOnTurnStartError},
		{method: "thread/resume", errCode: lostSessionCode, errMessage: lostOnResumeMessage},
		{method: "thread/start", result: `{"thread":{"id":"t-2"}}`},
		{method: "turn/start", result: `{"turn":{"id":"turn-9"}}`},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	origins := &recordedOrigins{}
	drv, err := apidriver.Start(ctx, d, sockPath, origins.claim)
	require.NoError(t, err)
	defer drv.Close()
	establishAtSpawn(t, ctx, drv)

	out, err := drv.Dispatch(ctx, "prompt", map[string]string{
		"session_id": "t-1", "cwd": "/work", "text": "are you there?",
	})
	require.NoError(t, err, "an unrecoverable thread must still deliver the prompt somewhere live")
	require.Equal(t, "t-2", out["session_id"])

	require.Len(t, *seen, 5, "expected start, failed turn, failed resume, fresh start, retried turn")
	require.Contains(t, (*seen)[3], `"sandbox":"workspace-write"`,
		"the replacement thread must be born with the same settings as the original")
	require.Contains(t, (*seen)[3], `"developerInstructions":"what came before"`,
		"the replacement thread must carry the handoff, or the chat silently loses its history")
	require.Contains(t, (*seen)[4], `"threadId":"t-2"`, "the retry must name the NEW thread")
	require.Contains(t, (*seen)[4], `"are you there?"`, "the user's prompt must actually be delivered")

	assert.Equal(t, []string{"t-1", "t-2"}, origins.ids,
		"the spawn-time thread and its replacement are both Crowbar's own, or the ingest reads "+
			"the replacement as a user-typed /clear")
	assert.Zero(t, origins.open, "every claim must be closed, including the failed resume's")
}

// TestRegression_UndeclaredErrorCodeIsStillFatal keeps the recovery narrow: it
// is driven by the codes codex.yaml declares and nothing else, so an ordinary
// protocol failure still surfaces as an error instead of silently abandoning a
// perfectly live session and starting a new one behind the user's back.
func TestRegression_UndeclaredErrorCodeIsStillFatal(t *testing.T) {
	sockPath, seen := scriptedServer(t, []scriptedCall{
		{method: "thread/start", result: `{"thread":{"id":"t-1"}}`},
		{method: "turn/start", errCode: -32602, errMessage: "invalid params"},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	drv, err := apidriver.Start(ctx, d, sockPath, nil)
	require.NoError(t, err)
	defer drv.Close()

	_, err = drv.Dispatch(ctx, "prompt", map[string]string{
		"session_id": "", "cwd": "/work", "text": "hi",
		"permission.sandbox": "workspace-write", "permission.approvalPolicy": "on-request",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid params")
	require.Len(t, *seen, 2, "a code the descriptor does not declare must not mint a new session")
}

// TestRegression_EstablishSessionFreshReportsTheSessionItMinted pins the root
// cause of the 2026-09-23 collab-agents transcript bleed. Only the RECOVERY
// paths used to claim what they produced; EstablishSession's ordinary success
// path — the one EVERY fresh api-transport chat's first message takes — ran its
// steps and returned, so the thread this connection had just minted was never
// recorded as ours. Downstream, the runner row keeps CurrentSession AND
// LaunchSessionID empty on this path (measured 47 of 49 real codex turn rows),
// which left the hook ingress with no way to tell the parent conversation from
// the child threads codex pushes down the very same websocket: the children's
// assistant messages landed in the user's transcript and a child's
// turn/completed closed the user's turn 83 seconds early.
func TestRegression_EstablishSessionFreshReportsTheSessionItMinted(t *testing.T) {
	sockPath, _ := scriptedServer(t, []scriptedCall{
		{method: "thread/start", result: `{"thread":{"id":"t-minted"}}`},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	origins := &recordedOrigins{}
	drv, err := apidriver.Start(ctx, d, sockPath, origins.claim)
	require.NoError(t, err)
	defer drv.Close()

	out, err := drv.EstablishSession(ctx, "prompt", map[string]string{
		"session_id": "", "cwd": "/work",
	})
	require.NoError(t, err)
	require.Equal(t, "t-minted", out["session_id"])

	assert.Equal(t, []string{"t-minted"}, origins.ids,
		"the thread this connection minted itself must be reported, or every child thread the "+
			"provider opens on it reads as the runner's own conversation")
	assert.Zero(t, origins.open, "the claim must be closed")
}

// A resume is an establish too: the conversation this connection loaded is the
// one it legitimately runs on, so it is claimed exactly as a fresh mint is.
// Resume's own steps declare no capture — session_id survives only because it
// went IN non-empty — which is why this asserts the id and not just the count.
func TestEstablishSession_ResumeReportsTheSessionItEntered(t *testing.T) {
	sockPath, _ := scriptedServer(t, []scriptedCall{
		{method: "thread/resume", result: `{"thread":{"id":"known-1"}}`},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	origins := &recordedOrigins{}
	drv, err := apidriver.Start(ctx, d, sockPath, origins.claim)
	require.NoError(t, err)
	defer drv.Close()

	_, err = drv.EstablishSession(ctx, "prompt", map[string]string{"session_id": "known-1", "cwd": "/work"})
	require.NoError(t, err)

	assert.Equal(t, []string{"known-1"}, origins.ids)
	assert.Zero(t, origins.open, "the claim must be closed")
}

// TestEstablishSession_AnAlreadyEstablishedConnectionOpensNoClaim keeps the
// claim tied to an actual establish. The short-circuit produces NO session — it
// hands back what this connection already remembers — and a claim opened there
// would sit open for nothing while biasing originatedSessions.has (runner
// package) to true, admitting whatever the provider announced in that window as
// Crowbar's own doing.
func TestEstablishSession_AnAlreadyEstablishedConnectionOpensNoClaim(t *testing.T) {
	sockPath, seen := scriptedServer(t, []scriptedCall{
		{method: "thread/start", result: `{"thread":{"id":"t-1"}}`},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)
	origins := &recordedOrigins{}
	drv, err := apidriver.Start(ctx, d, sockPath, origins.claim)
	require.NoError(t, err)
	defer drv.Close()

	_, err = drv.EstablishSession(ctx, "prompt", map[string]string{"session_id": "", "cwd": "/work"})
	require.NoError(t, err)
	require.Equal(t, 1, origins.claims, "the first establish claims exactly once")

	_, err = drv.EstablishSession(ctx, "prompt", map[string]string{"session_id": "t-1", "cwd": "/work"})
	require.NoError(t, err)

	require.Len(t, *seen, 1, "only the first EstablishSession may reach the wire")
	assert.Equal(t, 1, origins.claims, "a connection that produces no session claims nothing")
	assert.Equal(t, []string{"t-1"}, origins.ids)
	assert.Zero(t, origins.open)
}
