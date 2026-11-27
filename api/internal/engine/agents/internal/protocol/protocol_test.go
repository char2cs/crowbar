package protocol_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/jsonrpc"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

func loadCodexAPIDescriptor(t *testing.T) *spec.Descriptor {
	t.Helper()
	raw, err := os.ReadFile("internal/descriptor/descriptors-v3/codex.yaml")
	require.NoError(t, err)
	d, err := descriptor.ParseV0(raw)
	require.NoError(t, err)
	return d
}

type scriptedCall struct {
	method string
	result string
}

func scriptedServer(t *testing.T, script []scriptedCall) (stdio protocol.APIStdio, seenParams *[]string) {
	t.Helper()
	seen := make([]string, 0, len(script))
	in, out, peer := testutil.StdioPipe(t)
	go func() {
		for i := -2; i < len(script); i++ {
			var req struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if peer.ReadJSON(&req) != nil {
				return
			}
			if i == -1 {
				continue
			}
			result := json.RawMessage(`{}`)
			if i >= 0 {
				assert.Equal(t, script[i].method, req.Method, "unexpected call reaching the wire")
				seen = append(seen, string(req.Params))
				result = json.RawMessage(script[i].result)
			}
			assert.NoError(t, peer.WriteJSON(map[string]any{"id": req.ID, "result": result}))
		}
	}()
	return protocol.APIStdio{In: in, Out: out}, &seen
}

func TestAPIConn_FacadeForwardsEveryCallThroughToTheUnderlyingDriver(t *testing.T) {
	stdio, seen := scriptedServer(t, []scriptedCall{
		{method: "thread/resume", result: `{"thread":{"id":"sid-1"}}`},
		{method: "thread/inject_items", result: `{}`},
		{method: "turn/start", result: `{"turn":{"id":"turn-1"}}`},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)

	conn, err := protocol.StartAPIDriver(ctx, d, stdio, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	out, err := conn.EstablishSession(ctx, "prompt", map[string]string{"session_id": "sid-1", "cwd": "/work"})
	require.NoError(t, err)
	assert.Equal(t, "sid-1", out["session_id"], "APIConn.EstablishSession must hand back what the driver resolved")

	err = conn.InjectAt(ctx, "context", map[string]string{
		"session_id": "sid-1", "context": "while you were away, this happened",
	})
	require.NoError(t, err)
	require.Len(t, *seen, 2, "APIConn.InjectAt must reach the wire as its own call")
	assert.Contains(t, (*seen)[1], `"threadId":"sid-1"`, "InjectAt must forward the session the caller named")
	assert.Contains(t, (*seen)[1], "while you were away, this happened", "and the context text itself")

	out, err = conn.Dispatch(ctx, "prompt", map[string]string{"session_id": "sid-1", "cwd": "/work", "text": "hi"})
	require.NoError(t, err)
	assert.Equal(t, "sid-1", out["session_id"], "APIConn.Dispatch must hand back what the driver resolved")
	require.Len(t, *seen, 3, "Dispatch on an already-established session skips straight to the action call")
}

func TestAPIConn_InjectAtUndeclaredMomentIsANoopThroughTheFacade(t *testing.T) {
	stdio, seen := scriptedServer(t, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	d := loadCodexAPIDescriptor(t)

	conn, err := protocol.StartAPIDriver(ctx, d, stdio, nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	err = conn.InjectAt(ctx, "resume", map[string]string{"session_id": "sid-1"})
	require.NoError(t, err)
	assert.Empty(t, *seen, "an undeclared moment must never reach the wire")
}

func TestStartAPIDriver_PropagatesTheUnderlyingStartFailure(t *testing.T) {
	in, out, peer := testutil.StdioPipe(t)
	require.NoError(t, peer.Close())
	d := loadCodexAPIDescriptor(t)

	conn, err := protocol.StartAPIDriver(t.Context(), d, protocol.APIStdio{In: in, Out: out}, nil)

	require.Error(t, err)
	assert.Nil(t, conn)
}

func TestCheckVersion_DelegatesToTheDescriptorsDeclaredRange(t *testing.T) {
	d := &spec.Descriptor{ProtocolVersion: &spec.VersionRange{Min: "1.0", Max: "1.9"}}

	assert.NoError(t, protocol.CheckVersion(d, "1.5"))
	assert.Error(t, protocol.CheckVersion(d, "2.0"),
		"a version outside the descriptor's declared range must be refused")
}

func TestAPIFailureReason_QuotesTheProvidersOwnRefusalThroughAnyWrapping(t *testing.T) {
	refused := fmt.Errorf("turn: %w", &jsonrpc.CallError{Method: "turn/start", Message: "rate limited"})
	detailed := &jsonrpc.CallError{Method: "turn/start", Message: "invalid params", Detail: "field model"}

	reason, ok := protocol.APIFailureReason(refused)
	assert.True(t, ok)
	assert.Equal(t, "rate limited", reason)

	reason, ok = protocol.APIFailureReason(detailed)
	assert.True(t, ok)
	assert.Equal(t, "invalid params: field model", reason)

	_, ok = protocol.APIFailureReason(errors.New("connection reset"))
	assert.False(t, ok, "a transport failure is not the provider's refusal")
}
