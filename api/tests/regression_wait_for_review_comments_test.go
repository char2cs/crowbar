//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// An agent blocked in wait_for_review_comments must be handed the comment a human
// posts through the ordinary threads route. The post may land before or after the
// tool has subscribed; both orders have to deliver it, which is the lost-wakeup
// property, so the test needs no synchronisation between the two.
func TestRegression_WaitForReviewCommentsReturnsAHumanComment(t *testing.T) {
	h := newHarness(t)
	writeThreadStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	_, runnerID := newThreadChat(t, h, imported)

	payload, err := json.Marshal(map[string]any{
		"token": runnerFile(t, h, imported, runnerID, "runner-token"),
		"rpc": json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":` +
			`{"name":"wait_for_review_comments","arguments":{"timeoutSeconds":100}}}`),
	})
	require.NoError(t, err)
	url := h.url + repoBase(imported) + "/chats/runners/" + runnerID + "/mcp"

	type reply struct {
		body []byte
		err  error
	}
	done := make(chan reply, 1)
	go func() {
		resp, err := h.server.Client().Post(url, "application/json", bytes.NewReader(payload))
		if err != nil {
			done <- reply{err: err}
			return
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		done <- reply{body: body, err: err}
	}()

	h.post(threadsBase(imported)+"/threads", map[string]any{
		"filePath": "README.md",
		"line":     3,
		"side":     "right",
		"body":     "please rename this helper",
	}, http.StatusCreated, nil)

	got := <-done
	require.NoError(t, got.err)
	require.Contains(t, string(got.body), "please rename this helper")
	require.Contains(t, string(got.body), "needs your attention")
}
