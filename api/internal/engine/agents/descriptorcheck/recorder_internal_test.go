//go:build unix

package descriptorcheck

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A CLI with native HTTP hooks posts them to the recorder's own loopback server,
// which keeps them as it keeps a relayed one: the event name and the raw payload.
func TestRecorder_RecordsAHookPostedToItsLoopbackServer(t *testing.T) {
	r, err := newRecorder(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { r.close(context.Background()) })

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		r.url+"/v0/projects/p/home/chats/runners/s/hooks/claude/tool_pre",
		strings.NewReader(`{"tool_name":"Bash"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, r.has("tool_pre"))
	records := r.records()
	require.Len(t, records, 1)
	assert.JSONEq(t, `{"tool_name":"Bash"}`, string(records[0].payload))
}

func TestRecorder_CloseWithoutAServerIsHarmless(t *testing.T) {
	assert.NotPanics(t, func() { recorder{}.close(t.Context()) })
}
