//go:build integration && unix

package scripted_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/tests/kit"
)

const wsProvider = "wsprov"

// wsDescriptor is a serve-less provider reached over a websocket: connect prints
// the endpoint, and every event it speaks is declared here.
const wsDescriptor = `schema_version: 0
id: wsprov
display_name: Socket Provider
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    delivery: websocket
    format: jsonrpc_text
    connect: [cat, %q]
    connect_map: { url: url }
    handshake: { call: hello }
    reconnect: { max: 3, backoff_ms: 1 }
spawn:
  cmd: cat
  interactive_required: true
session:
  refused: { exit_code: 1, rpc_code: -32600, needles: ["no such session"] }
  persists: turn_end
surfaces:
  chat: { channel: api, start_here: true }
events:
  session_start:
    required: [session_id]
    api: { in: session/started, map: { session_id: session_id } }
  user_prompt:
    required: [session_id]
    api: { in: item/user, map: { session_id: session_id, message: text } }
  message_delta:
    in: item/delta
    map: { session_id: session_id, message_id: item_id, text: text }
  tool_pre:
    required: [session_id, tool_id, tool_name]
    api: { in: tool/started, map: { session_id: session_id, tool_id: id, tool_name: name } }
  tool_post:
    required: [session_id, tool_id, tool_name]
    api: { in: tool/done, map: { session_id: session_id, tool_id: id, tool_name: name } }
  turn_stop:
    required: [session_id]
    api: { in: turn/done, map: { session_id: session_id, message: text } }
  turn_failed:
    required: [session_id, reason]
    api: { in: turn/failed, map: { session_id: session_id, reason: reason } }
  permission:
    required: [session_id, tool_name]
    api:
      ask: [approval/request]
      map: { session_id: session_id, tool_name: tool }
    reply:
      allow: '{"decision":"accept"}'
      deny: '{"decision":"decline"}'
  prompt:
    fresh:
      - { call: session/new, send: { cwd: "{cwd}" }, capture: { session_id: id } }
    resume:
      - { call: session/resume, send: { id: "{session_id}" } }
    action:
      - { call: turn/send, send: { id: "{session_id}", text: "{text}", call: "{call_id}" }, capture: { turn_id: turn } }
  interrupt:
    out: turn/cancel
    send: { id: "{session_id}", turn: "{turn_id}" }
`

var callIDRE = regexp.MustCompile(`^[0-9a-f]{32}$`)

// withoutCallID returns a turn/send's params minus its per-call id, which it checks is fresh-looking.
func withoutCallID(t *testing.T, params json.RawMessage) string {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(params, &m))
	id, _ := m["call"].(string)
	assert.Regexp(t, callIDRE, id)
	delete(m, "call")
	out, err := json.Marshal(m)
	require.NoError(t, err)
	return string(out)
}

// socketRig is a daemon whose only provider dials a fake socket.
type socketRig struct {
	*rig
	ws       *kit.FakeWS
	endpoint string
	calls    chan wireMsg
	reject   sync.Map // method -> error message the socket answers it with
}

// wireMsg is one message the daemon wrote to the socket.
type wireMsg struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
}

func newSocketRig(t *testing.T) *socketRig {
	t.Helper()
	s := &socketRig{ws: kit.NewFakeWS(t), endpoint: filepath.Join(t.TempDir(), "endpoint.json"), calls: make(chan wireMsg, 64)}
	s.offerEndpoint(t)
	s.rig = newRigWith(t, func(t *testing.T, home, _ string) {
		require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(home, "descriptors", wsProvider+".yaml"),
			fmt.Appendf(nil, wsDescriptor, s.endpoint), 0o600))
	})
	go s.answer(t.Context())
	return s
}

// offerEndpoint publishes a fresh single-use ticket for the next (re)connect.
func (s *socketRig) offerEndpoint(t *testing.T) {
	t.Helper()
	doc, err := json.Marshal(map[string]string{"url": s.ws.URL(s.ws.NewTicket())})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(s.endpoint, doc, 0o600))
}

// answer replies to every call the daemon makes and records all it writes.
func (s *socketRig) answer(ctx context.Context) {
	results := map[string]string{"session/new": `{"id":"sess-1"}`, "turn/send": `{"turn":"turn-1"}`}
	for {
		select {
		case <-ctx.Done():
			return
		case raw := <-s.ws.Frames:
			var f wireMsg
			if json.Unmarshal(raw, &f) != nil {
				continue
			}
			s.calls <- f
			if f.Method == "" || len(f.ID) == 0 {
				continue
			}
			if msg, ok := s.reject.Load(f.Method); ok {
				_ = s.ws.Send(fmt.Appendf(nil, `{"jsonrpc":"2.0","id":%s,"error":{"code":4001,"message":%q}}`, f.ID, msg))
				continue
			}
			result, ok := results[f.Method]
			if !ok {
				result = `{}`
			}
			_ = s.ws.Send(fmt.Appendf(nil, `{"jsonrpc":"2.0","id":%s,"result":%s}`, f.ID, result))
		}
	}
}

// next is the next frame the daemon wrote.
func (s *socketRig) next(t *testing.T) wireMsg {
	t.Helper()
	select {
	case f := <-s.calls:
		return f
	case <-time.After(20 * time.Second):
		require.FailNow(t, "the daemon wrote nothing to the socket")
		return wireMsg{}
	}
}

// nextMethod skips replies and returns the next call named method.
func (s *socketRig) nextMethod(t *testing.T, method string) wireMsg {
	t.Helper()
	for {
		if f := s.next(t); f.Method == method {
			return f
		}
	}
}

func (s *socketRig) push(t *testing.T, msg string) {
	t.Helper()
	require.NoError(t, s.ws.Send([]byte(msg)))
}

// A websocket descriptor with no serve connects, runs a whole turn over the
// socket, answers a server-initiated approval over it, and ends cleanly.
func TestRegression_WebsocketDeliveryRunsAChatTurn(t *testing.T) {
	s := newSocketRig(t)
	chatID := s.spawn(wsProvider)
	assert.Equal(t, "hello", s.nextMethod(t, "hello").Method)
	assert.Contains(t, string(s.nextMethod(t, "session/new").Params), `"cwd"`, "the fresh session names its workspace")

	require.NoError(t, s.send(chatID, "hi there"))
	send := s.nextMethod(t, "turn/send")
	assert.JSONEq(t, `{"id":"sess-1","text":"hi there"}`, withoutCallID(t, send.Params))

	s.push(t, `{"method":"session/started","params":{"session_id":"sess-1"}}`)
	s.push(t, `{"method":"item/user","params":{"session_id":"sess-1","text":"hi there"}}`)
	s.push(t, `{"method":"item/delta","params":{"session_id":"sess-1","item_id":"m1","text":"hel"}}`)
	s.push(t, `{"method":"tool/started","params":{"session_id":"sess-1","id":"t1","name":"Bash"}}`)
	s.push(t, `{"method":"tool/done","params":{"session_id":"sess-1","id":"t1","name":"Bash"}}`)
	s.push(t, `{"id":77,"method":"approval/request","params":{"session_id":"sess-1","tool":"Bash"}}`)

	var choice domain.ActivityChoice
	s.eventually(func() bool {
		pending := s.choices(chatID)
		if len(pending) == 1 {
			choice = pending[0]
		}
		return len(pending) == 1
	}, "the server's approval request never reached the chat")
	require.NoError(t, s.d.app.Usecases.AgentAnswer.AnswerChoice(
		context.Background(), chatID, choice.ID, []string{allowOption(choice)}, "", nil))
	answer := s.next(t)
	assert.JSONEq(t, "77", string(answer.ID), "the answer rides the request's own id")
	assert.JSONEq(t, `{"decision":"accept"}`, string(answer.Result))

	s.push(t, `{"method":"turn/done","params":{"session_id":"sess-1","text":"hello"}}`)
	s.eventually(func() bool { return slices.Contains(s.said(chatID), "assistant: hello") && !s.working(chatID) },
		"the turn never closed")
	assert.Contains(t, s.said(chatID), "user: hi there")
	calls := kit.Bodies[ledger.ToolCall](s.rows(chatID), ledger.KindToolCall)
	require.NotEmpty(t, calls)
	assert.True(t, ledger.Fold(chatID, s.rows(chatID)).HasToolPhase("", "t1", ledger.ToolPost), "the tool call closed")
	assert.NotEmpty(t, kit.Bodies[ledger.SessionBound](s.rows(chatID), ledger.KindSessionBound), "the session was bound")

	require.NoError(t, s.runners().StopChat(context.Background(), chatID))
	assert.Equal(t, domain.AgentExitStopped, s.exitReason(chatID))
}

// A socket that drops mid-turn fails that turn in the ledger, then redials with a
// fresh ticket and re-enters the same session without replaying the prompt.
func TestRegression_WebsocketDropFailsTheTurnAndResumesTheSession(t *testing.T) {
	s := newSocketRig(t)
	chatID := s.spawn(wsProvider)
	require.NoError(t, s.send(chatID, "first"))
	s.nextMethod(t, "turn/send")
	s.push(t, `{"method":"item/user","params":{"session_id":"sess-1","text":"first"}}`)
	s.push(t, `{"method":"item/delta","params":{"session_id":"sess-1","item_id":"m1","text":"par"}}`)
	s.eventually(func() bool { return s.working(chatID) }, "the turn never opened")

	s.offerEndpoint(t)
	s.ws.Drop()

	s.eventually(func() bool {
		return len(kit.Bodies[ledger.TurnFailed](s.rows(chatID), ledger.KindTurnFailed)) == 1
	}, "a dropped socket left the turn open")
	s.nextMethod(t, "hello")
	assert.JSONEq(t, `{"id":"sess-1"}`, string(s.nextMethod(t, "session/resume").Params))

	require.NoError(t, s.send(chatID, "second"))
	send := s.nextMethod(t, "turn/send")
	assert.JSONEq(t, `{"id":"sess-1","text":"second"}`, withoutCallID(t, send.Params), "the first prompt was not replayed")
	s.push(t, `{"method":"item/user","params":{"session_id":"sess-1","text":"second"}}`)
	s.push(t, `{"method":"turn/done","params":{"session_id":"sess-1","text":"ok"}}`)
	s.eventually(func() bool { return slices.Contains(s.said(chatID), "assistant: ok") && !s.working(chatID) },
		"the resumed session never answered")
}

// A chat whose only channel is the socket has no argv to carry a prompt or a
// session: after its connection is gone, a send redials, resumes the bound
// session by the descriptor's own step, and delivers over the new connection.
func TestRegression_WebsocketDormantChatResumesItsSessionToDeliverAPrompt(t *testing.T) {
	s := newSocketRig(t)
	chatID := s.spawn(wsProvider)
	require.NoError(t, s.send(chatID, "first"))
	s.nextMethod(t, "turn/send")
	s.push(t, `{"method":"item/user","params":{"session_id":"sess-1","text":"first"}}`)
	s.push(t, `{"method":"turn/done","params":{"session_id":"sess-1","text":"one"}}`)
	s.eventually(func() bool { return slices.Contains(s.said(chatID), "assistant: one") && !s.working(chatID) },
		"the first turn never closed")

	require.NoError(t, s.runners().StopChat(context.Background(), chatID))
	s.offerEndpoint(t)

	require.NoError(t, s.send(chatID, "second"))
	s.nextMethod(t, "hello")
	assert.JSONEq(t, `{"id":"sess-1"}`, string(s.nextMethod(t, "session/resume").Params))
	send := s.nextMethod(t, "turn/send")
	assert.JSONEq(t, `{"id":"sess-1","text":"second"}`, withoutCallID(t, send.Params))
}

// A provider that refuses to open the session tells the chat why, in the chat's
// own feed, instead of leaving the send to fail as an unsupported prompt.
func TestRegression_WebsocketRefusedSessionTellsTheChatWhy(t *testing.T) {
	s := newSocketRig(t)
	s.reject.Store("session/new", "storage unavailable")
	chatID := s.spawn(wsProvider)

	err := s.send(chatID, "hello")
	assert.NotErrorIs(t, err, problem.ErrUnprocessable, "a failed connection is not an unsupported prompt")
	var details []string
	for _, m := range kit.Bodies[ledger.Marker](s.rows(chatID), ledger.KindMarker) {
		details = append(details, m.Detail)
	}
	assert.Contains(t, strings.Join(details, "\n"), "storage unavailable")
}

// Every prompt delivery mints its own {call_id}, so a re-send never reuses an id the
// provider already admitted.
func TestRegression_WebsocketEachPromptCarriesAFreshCallID(t *testing.T) {
	s := newSocketRig(t)
	chatID := s.spawn(wsProvider)
	var ids []string
	for _, text := range []string{"one", "two"} {
		require.NoError(t, s.send(chatID, text))
		var m map[string]any
		require.NoError(t, json.Unmarshal(s.nextMethod(t, "turn/send").Params, &m))
		id, _ := m["call"].(string)
		ids = append(ids, id)
		s.push(t, fmt.Sprintf(`{"method":"item/user","params":{"session_id":"sess-1","text":%q}}`, text))
		s.push(t, `{"method":"turn/done","params":{"session_id":"sess-1","text":"done-`+text+`"}}`)
		s.eventually(func() bool { return slices.Contains(s.said(chatID), "assistant: done-"+text) && !s.working(chatID) },
			"the turn never closed")
	}
	assert.Regexp(t, callIDRE, ids[0])
	assert.Regexp(t, callIDRE, ids[1])
	assert.NotEqual(t, ids[0], ids[1])
}

// A prompt the provider refuses leaves the chat's stop notice carrying the provider's own
// reason, not a guess about its CLI.
func TestRegression_WebsocketRefusedPromptRecordsItsReasonOnTheExit(t *testing.T) {
	s := newSocketRig(t)
	chatID := s.spawn(wsProvider)
	require.NoError(t, s.runners().StopChat(context.Background(), chatID))
	s.offerEndpoint(t)
	s.reject.Store("turn/send", "message too long")
	_ = s.send(chatID, "hello")

	s.eventually(func() bool {
		for _, e := range kit.Bodies[ledger.RunnerExited](s.rows(chatID), ledger.KindRunnerExited) {
			if strings.Contains(e.Detail, "message too long") {
				return true
			}
		}
		return false
	}, "no exit row carried the provider's reason")
}
