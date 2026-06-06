package server

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/protocol"
)

// TestHelperProcess is not a real test: it is re-executed as the language
// server subprocess. It reads one framed request from stdin and writes back a
// framed response echoing the id with a fixed result.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("CROWBAR_LSP_HELPER") != "1" {
		t.Skip("helper process: not invoked directly")
	}
	r := bufio.NewReader(os.Stdin)
	payload, err := protocol.ReadMessage(r)
	if err != nil {
		os.Exit(1)
	}
	var req protocol.Request
	_ = json.Unmarshal(payload, &req)

	resp := protocol.Response{
		JSONRPC: "2.0",
		ID:      &req.ID,
		Result:  json.RawMessage(`{"ok":true}`),
	}
	out, _ := json.Marshal(resp)
	_ = protocol.WriteMessage(os.Stdout, out)
	os.Exit(0)
}

func TestProcessTransport_CloseReapsProcess(t *testing.T) {
	t.Setenv("CROWBAR_LSP_HELPER", "1")

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	tr, err := newProcessTransport(cmd)
	require.NoError(t, err)

	require.NoError(t, tr.Close())
	require.NotNil(t, cmd.ProcessState, "process must be reaped (Wait called) after Close")

	// Close is idempotent and must not panic on an already-reaped process.
	require.NoError(t, tr.Close())
}

func TestServer_NewSpawnsRealProcess(t *testing.T) {
	t.Setenv("CROWBAR_LSP_HELPER", "1")

	srv, err := New(
		os.Args[0],
		[]string{"-test.run=TestHelperProcess"},
		"",
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	raw, err := srv.Request(ctx, "textDocument/hover", map[string]any{"x": 1})
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":true}`, string(raw))
}
