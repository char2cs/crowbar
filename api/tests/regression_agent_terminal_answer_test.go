//go:build integration

package tests

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The stub paints a trust prompt and blocks until it reads a line, then clears
// the screen the way a real CLI repaints once its modal is dismissed.
const trustStubProviderDescriptorYAML = `id: truststub
spawn:
  cmd: "sh"
  args: ["-c", "printf 'Trust this folder?\\n'; read answer; printf '\\033[2J\\033[H'; exec cat"]
  interactive_required: true
terminal_prompts:
  - kind: workspace_trust
    needle: "Trust this folder?"
    answers:
      - id: accept
        label: "Trust workspace"
        keys: "\r"
runtime:
  transport: hooks
  hooks:
    format: json
`

type terminalWaitDetail struct {
	TerminalWait *struct {
		Kind    string `json:"kind"`
		Answers []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
		} `json:"answers"`
	} `json:"terminalWait"`
}

func readTerminalWait(t *testing.T, h *harness, imported importedRepo, chatID string) terminalWaitDetail {
	t.Helper()
	var d terminalWaitDetail
	h.get(repoBase(imported)+"/chats/"+chatID, &d)
	return d
}

// TestRegression_TerminalPromptIsAnswerableFromTheChat proves a provider blocked
// on its trust modal can be unblocked without opening the terminal: the descriptor's
// answer is offered on the chat, posting it writes its keys to the PTY, and the
// detector clears the wait once the screen no longer shows the prompt.
func TestRegression_TerminalPromptIsAnswerableFromTheChat(t *testing.T) {
	h := newHarness(t)
	dir := filepath.Join(h.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "truststub.yaml"),
		[]byte(stubDescriptor(trustStubProviderDescriptorYAML)), 0o644))
	imported := importWritableWorkspace(t, h)

	var created struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats",
		map[string]string{"provider": "truststub", "workspaceId": imported.workspaceID},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	answerURL := repoBase(imported) + "/chats/" + created.ID + "/terminal-wait/answer"

	require.Eventually(t, func() bool {
		return readTerminalWait(t, h, imported, created.ID).TerminalWait != nil
	}, 20*time.Second, 100*time.Millisecond, "the blocked provider must surface as a terminal wait")

	wait := readTerminalWait(t, h, imported, created.ID).TerminalWait
	assert.Equal(t, "workspace_trust", wait.Kind)
	require.Len(t, wait.Answers, 1)
	assert.Equal(t, "accept", wait.Answers[0].ID)
	assert.Equal(t, "Trust workspace", wait.Answers[0].Label)

	h.raw(http.MethodPost, answerURL, map[string]string{"answerId": "nope"}, http.StatusBadRequest).Body.Close()
	h.raw(http.MethodPost, answerURL, map[string]string{"answerId": "accept"}, http.StatusOK).Body.Close()

	require.Eventually(t, func() bool {
		return readTerminalWait(t, h, imported, created.ID).TerminalWait == nil
	}, 20*time.Second, 100*time.Millisecond, "the wait must clear once the provider moved past its prompt")

	h.raw(http.MethodPost, answerURL, map[string]string{"answerId": "accept"}, http.StatusConflict).Body.Close()
}
