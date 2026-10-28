//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// awaitOperation dials the operations stream and reads frames until done holds
// for operation id. The snapshot sent on connect carries the operation's current
// row, so a change that happened before the dial is never missed.
func awaitOperation(
	t *testing.T,
	h *harness,
	id string,
	done func(domain.Operation) bool,
) domain.Operation {
	t.Helper()
	conn := h.dial("/v0/operations")
	var got domain.Operation
	readUntil(t, conn, func(frame map[string]any) bool {
		if frame["id"] != id {
			return false
		}
		raw, err := json.Marshal(frame)
		require.NoError(t, err)
		got = domain.Operation{}
		require.NoError(t, json.Unmarshal(raw, &got))
		return done(got)
	})
	require.NoError(t, conn.Close())
	return got
}

func operationSettled(op domain.Operation) bool {
	return op.State == domain.OpSucceeded || op.State == domain.OpFailed
}

// startOperation POSTs body (nil for none) under a fresh Idempotency-Key and
// returns the operation the 202 answers with.
func startOperation(
	t *testing.T,
	h *harness,
	path string,
	body any,
) domain.Operation {
	t.Helper()
	key := uuid.NewString()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, h.url+path, reader)
	require.NoError(t, err)
	req.Header.Set("Idempotency-Key", key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusAccepted, resp.StatusCode, path)
	var env struct {
		Data struct {
			Operation domain.Operation `json:"operation"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	require.Equal(t, key, env.Data.Operation.ID, "the operation id is the client's key")
	return env.Data.Operation
}

func TestOperations_SnapshotCarriesSettledOperationsAcrossARestart(t *testing.T) {
	h := newHarness(t)
	push := failingPush(t, h)
	op := startOperation(t, h, "/v0/chats/"+push.chatID+"/git/push", nil)
	failed := awaitOperation(t, h, op.ID, operationSettled)
	require.Equal(t, domain.OpFailed, failed.State)

	h.crash()
	again := newHarnessAt(t, h.home)

	restored := awaitOperation(t, again, op.ID, func(domain.Operation) bool { return true })
	require.Equal(t, domain.OpFailed, restored.State)
	require.Equal(t, failed.Error, restored.Error)
	require.Equal(t, failed.Version, restored.Version)
}
