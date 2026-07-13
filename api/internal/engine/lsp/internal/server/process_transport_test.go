package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"

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
	tr, err := newProcessTransport(cmd, nil)
	require.NoError(t, err)

	require.NoError(t, tr.Close())
	require.NotNil(t, cmd.ProcessState, "process must be reaped (Wait called) after Close")

	// Close is idempotent and must not panic on an already-reaped process.
	require.NoError(t, tr.Close())
}

// TestProcessTransport_NaturalExitReapsAndFiresOnExit proves R10 at the
// transport layer (mirroring the terminal session natural-exit reap test): a
// process that exits on its own is reaped via cmd.Wait() — ProcessState is set —
// and the onExit callback fires exactly once so the manager can evict the dead
// pool entry. The reaper, not Close(), owns the single Wait here.
func TestProcessTransport_NaturalExitReapsAndFiresOnExit(t *testing.T) {
	t.Setenv("CROWBAR_LSP_HELPER", "1")

	exited := make(chan struct{})
	var once sync.Once

	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	tr, err := newProcessTransport(cmd, func() {
		once.Do(func() { close(exited) })
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tr.Close() })

	// The helper reads one framed request, replies, then exits on its own
	// (os.Exit(0)). Send a request so it proceeds to exit, and drain its reply so
	// the reaper observes the exit promptly.
	go func() { _, _ = io.Copy(io.Discard, tr) }()
	req := protocol.Request{JSONRPC: "2.0", ID: 1, Method: "ping"}
	out, marshalErr := json.Marshal(req)
	require.NoError(t, marshalErr)
	require.NoError(t, protocol.WriteMessage(tr, out))

	// Block on the transport's own onExit callback — the real signal that the
	// reaper observed the child's exit. No deadline: if the reaper never fires,
	// `go test -timeout` reports it with the goroutine dump.
	<-exited

	require.NotNil(t, cmd.ProcessState, "natural exit must reap the child via cmd.Wait()")
	assert.True(t, cmd.ProcessState.Exited(), "child process must have exited")
}

// TestProcessTransport_CloseSuppressesOnExit verifies that a deliberate Close
// (manager Release/Shutdown/Replay) does NOT fire onExit — only a genuine
// self-exit does.
func TestProcessTransport_CloseSuppressesOnExit(t *testing.T) {
	t.Setenv("CROWBAR_LSP_HELPER", "1")

	var fired int32
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	tr, err := newProcessTransport(cmd, func() { atomic.AddInt32(&fired, 1) })
	require.NoError(t, err)

	// Close before the process finishes on its own; onExit must be suppressed.
	require.NoError(t, tr.Close())
	require.NotNil(t, cmd.ProcessState, "Close must reap the process")
	assert.Equal(t, int32(0), atomic.LoadInt32(&fired), "deliberate Close must not fire onExit")
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

	// The request blocks on the real LSP response framed back over the child's
	// stdout. No request deadline: a server that never answers is a bug, and
	// `go test -timeout` is the backstop for it.
	raw, err := srv.Request(context.Background(), "textDocument/hover", map[string]any{"x": 1})
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":true}`, string(raw))
}
