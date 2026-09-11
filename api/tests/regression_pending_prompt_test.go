//go:build integration

package tests

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
)

// livestubPromptSubmitYAML extends writeLiveStubProviderDescriptor's stub
// (agent_working_overlay_test.go) with the presentation.prompt_submit and
// session.resume blocks POST .../prompts requires and that shared descriptor
// never declares — no earlier user of createLiveStubChat submits a real
// prompt over HTTP, only postAgentHook. Without them SubmitPrompt refuses
// every text with 422 (runner.ErrPromptUnsupported), so this test carries
// its own override rather than editing the shared harness file.
//
// spawn.cmd is "sh -c cat", not bare "cat": prompt_submit's fresh/resume
// steps place {message} as a trailing positional argv entry (mirroring
// descriptors-v3/claude.yaml), and "cat -- <message>" would try to OPEN a
// file named after the message and exit immediately, breaking the "stays
// alive on its PTY" property the shared stub relies on. The extra argv lands
// on $0/$1 of the sh -c script, which the literal script "cat" never
// references, so /bin/cat still runs bare, reading stdin, alive as before.
const livestubPromptSubmitYAML = `id: livestub
spawn:
  cmd: "sh"
  args: ["-c", "cat"]
  interactive_required: true
session:
  resume: { arg: "--resume {id}" }
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
runtime:
  transport: hooks
  hooks:
    format: json
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh:
      - pass_arg: { positional: "--" }
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "--" }
      - pass_arg: { positional: "{message}" }
`

func writeLiveStubProviderDescriptorWithPromptSubmit(
	t *testing.T,
	h *harness,
) {
	t.Helper()
	dir := filepath.Join(h.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "livestub.yaml"), []byte(livestubPromptSubmitYAML), 0o644))
}

// TestRegression_PendingPromptSurvivesAFrontendThatForgotItsOwnQueue proves
// the fix for "user's turns after some time of idle is lost, and does not
// record anywhere": a submitted prompt's literal text is recoverable from
// the backend even when the client that sent it has no memory of having
// done so — not merely a hash it cannot use to show the user anything back.
//
// This is the first test in this suite to drive the real POST .../prompts
// route against a live-stub chat (createLiveStubChat) rather than a
// hooks-simulated one. See livestubPromptSubmitYAML's doc comment for what
// that turned out to require and why: the shared stub descriptor
// (writeLiveStubProviderDescriptor) cannot dispatch a prompt at all over
// this route, a 422 for any text, so this test supplies its own descriptor
// override instead.
//
// The GET below is a single, unretried read straight after the POST
// returns, with no user_prompt hook ever fired to confirm acceptance — the
// point of the test is to catch the record BEFORE anything could confirm
// it, so waiting for settlement here would defeat the assertion rather than
// stabilise it.
func TestRegression_PendingPromptSurvivesAFrontendThatForgotItsOwnQueue(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptorWithPromptSubmit(t, h)
	imported := importWritableWorkspace(t, h)
	chatID, _ := createLiveStubChat(t, h, imported)

	const submittedText = "please rename this function to something clearer"

	resp := h.raw(http.MethodPost, wsBase(imported)+"/chats/"+chatID+"/prompts",
		map[string]string{"text": submittedText, "clientRequestId": "11111111-1111-1111-1111-111111111111"},
		http.StatusOK,
	)
	_ = resp.Body.Close()

	var got struct {
		Text  string `json:"text"`
		State string `json:"state"`
	}
	h.get(wsBase(imported)+"/chats/"+chatID+"/pending-prompt", &got)

	assert.Equal(t, submittedText, got.Text,
		"the recovered record must carry the literal submitted text, not merely a hash — "+
			"the whole point of the fix is giving the user their words back")
	assert.Equal(t, agentjournal.PromptStateSpawned, got.State,
		"the replacement runner was spawned and its identity durably recorded, but nothing "+
			"has fired the user_prompt hook that would confirm the provider accepted it, so the "+
			"record must still read as spawned — never accepted")
}
