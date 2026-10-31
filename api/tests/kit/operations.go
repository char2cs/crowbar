//go:build integration

package kit

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// AwaitOperation decodes the operation a 202 answered with (closing resp's body)
// and holds the stream's projects scope until it settles. The scope's snapshot
// carries its current row, so an operation that settled before the dial is seen.
func (e *Env) AwaitOperation(
	t *testing.T,
	resp *http.Response,
	timeout time.Duration,
) domain.Operation {
	t.Helper()
	RequireStatus(t, resp, http.StatusAccepted)
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	var env struct {
		Data struct {
			Operation domain.Operation `json:"operation"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &env), "AwaitOperation body: %s", string(raw))
	id := env.Data.Operation.ID
	require.NotEmpty(t, id, "AwaitOperation: the 202 names no operation; body: %s", string(raw))

	w := Dial(t, e.dialer, wsURL(e.URL, "/v0/stream"))
	w.WriteJSON(t, stream.Request{Op: stream.OpSub, Scope: "projects"})
	var got domain.Operation
	w.ReadUntil(t, timeout, func(frame map[string]any) bool {
		if frame["t"] != stream.FrameEntity || frame["kind"] != string(stream.KindOperation) || frame["id"] != id {
			return false
		}
		encoded, err := json.Marshal(frame["data"])
		require.NoError(t, err)
		got = domain.Operation{}
		require.NoError(t, json.Unmarshal(encoded, &got))
		return got.State == domain.OpSucceeded || got.State == domain.OpFailed
	})
	return got
}
