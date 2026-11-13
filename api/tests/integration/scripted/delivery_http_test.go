//go:build integration && unix

package scripted_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/tests/kit"
)

const httpProvider = "httpprov"

// httpDescriptor is a serve-less provider reached over http: named requests for
// what Crowbar says, one event stream for what the provider does.
const httpDescriptor = `schema_version: 0
id: httpprov
display_name: Http Provider
runtime:
  transport: api
  api:
    protocol: rest
    delivery: http
    format: sse
    static_url: %s
    reconnect: { max: 3, backoff_ms: 1 }
    requests:
      create_run: { method: POST, path: /runs, body: { cwd: "{cwd}" }, capture: { run_id: id } }
      get_run: { method: GET, path: "/runs/{session_id}", capture: { run_id: id } }
      send_message: { method: POST, path: "/runs/{run_id}/messages", body: { text: "{text}" } }
      cancel_run: { method: POST, path: "/runs/{run_id}/cancel" }
      approve: { method: POST, path: "/runs/{run_id}/approvals/{ask_id}", body: { decision: "{decision}" } }
    stream: { path: "/runs/{run_id}/events" }
spawn:
  cmd: cat
  interactive_required: true
session:
  resume: { arg: "resume {id}" }
  refused: { exit_code: 1, rpc_code: -32600, needles: ["no such session"] }
  persists: turn_end
surfaces:
  chat: { channel: api, start_here: true }
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh: [{ pass_arg: { positional: "{message}" } }]
    resume: [{ pass_arg: { positional: "{message}" } }]
events:
  session_start:
    required: [session_id]
    api: { in: session/started, map: { session_id: session_id } }
  user_prompt:
    required: [session_id]
    api: { in: item/user, map: { session_id: session_id, message: text } }
  turn_stop:
    required: [session_id]
    api: { in: turn/done, map: { session_id: session_id, message: text } }
  turn_failed:
    required: [session_id, reason]
    api: { in: turn/failed, map: { session_id: session_id, reason: reason } }
  permission:
    required: [session_id, tool_name]
    reply_request: approve
    api:
      ask: [approval/request]
      map: { session_id: session_id, tool_name: tool, ask_id: approval_id }
    reply:
      allow: '{"decision":"allow"}'
      deny: '{"decision":"deny"}'
  prompt:
    fresh:
      - { call: create_run, send: { cwd: "{cwd}" }, capture: { session_id: id } }
    resume:
      - { call: get_run }
    action:
      - { call: send_message, send: { text: "{text}" } }
  interrupt:
    out: cancel_run
    send: { id: "{session_id}" }
`

// An http descriptor with no serve connects, runs a turn through its requests and
// stream, answers an approval through its reply request, and cancels through its
// interrupt request; a dropped stream resumes from the last event it delivered.
func TestRegression_HTTPDeliveryRunsAChatTurn(t *testing.T) {
	srv := kit.NewFakeHTTP(t)
	srv.Route("POST", "/runs", 200, `{"id":"run-1"}`)
	srv.Route("POST", "/runs/run-1/messages", 200, `{}`)
	srv.Route("POST", "/runs/run-1/approvals/ask-9", 200, `{}`)
	srv.Route("POST", "/runs/run-1/cancel", 200, `{}`)
	srv.StreamAt("/runs/run-1/events")
	r := newRigWith(t, func(t *testing.T, home, _ string) {
		require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(home, "descriptors", httpProvider+".yaml"),
			fmt.Appendf(nil, httpDescriptor, srv.URL), 0o600))
	})
	chatID := r.spawn(httpProvider)
	<-srv.StreamOpened()
	sent := func(method, path string) func() bool {
		return func() bool {
			return slices.ContainsFunc(srv.Requests(), func(q kit.RecordedRequest) bool { return q.Method == method && q.Path == path })
		}
	}

	require.NoError(t, r.send(chatID, "hi there"))
	r.eventually(sent("POST", "/runs/run-1/messages"), "the prompt never reached the provider")
	srv.Event("1", "session/started", `{"session_id":"run-1"}`)
	srv.Event("2", "item/user", `{"session_id":"run-1","text":"hi there"}`)
	srv.Event("3", "approval/request", `{"session_id":"run-1","tool":"Bash","approval_id":"ask-9"}`)
	var choice domain.ActivityChoice
	r.eventually(func() bool {
		pending := r.choices(chatID)
		if len(pending) == 1 {
			choice = pending[0]
		}
		return len(pending) == 1
	}, "the provider's approval request never reached the chat")
	require.NoError(t, r.d.app.Usecases.AgentAnswer.AnswerChoice(
		context.Background(), chatID, choice.ID, []string{allowOption(choice)}, "", nil))
	r.eventually(sent("POST", "/runs/run-1/approvals/ask-9"), "the answer never reached the provider")
	for _, q := range srv.Requests() {
		if q.Path == "/runs/run-1/approvals/ask-9" {
			assert.JSONEq(t, `{"decision":"allow"}`, q.Body)
		}
	}

	srv.DropStream()
	<-srv.StreamOpened()
	srv.Event("4", "turn/done", `{"session_id":"run-1","text":"hello"}`)
	r.eventually(func() bool { return slices.Contains(r.said(chatID), "assistant: hello") && !r.working(chatID) },
		"the turn never closed across the stream drop")
	var resumedFrom []string
	for _, q := range srv.Requests() {
		if q.Method == "GET" {
			resumedFrom = append(resumedFrom, q.LastEventID)
		}
	}
	assert.Equal(t, []string{"", "3"}, resumedFrom, "the stream resumed from the last event it saw")
	assert.NotEmpty(t, kit.Bodies[ledger.SessionBound](r.rows(chatID), ledger.KindSessionBound))

	require.NoError(t, r.send(chatID, "again"))
	srv.Event("5", "item/user", `{"session_id":"run-1","text":"again"}`)
	r.eventually(func() bool { return r.working(chatID) }, "the second turn never opened")
	require.NoError(t, r.runners().StopChat(context.Background(), chatID))
	r.eventually(sent("POST", "/runs/run-1/cancel"), "the stop never fired the interrupt request")
}
