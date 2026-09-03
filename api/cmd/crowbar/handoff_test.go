package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// newHandoffTestSocket stands up a unix listener under a short /tmp directory
// (t.TempDir() nests under the long test name and overflows macOS's 104-byte
// sun_path limit) serving handler for every request, and returns the socket
// path plus a cleanup func.
func newHandoffTestSocket(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	tmpDir, err := os.MkdirTemp("/tmp", "handoff")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })
	sock := filepath.Join(tmpDir, "h.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

	srv := &http.Server{Handler: handler}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return sock
}

// TestRunHandoffDump_PrintsHandoff proves runHandoffDump GETs the
// workspace-nested /v0/projects/<p>/repos/<r>/workspaces/<w>/chats/<id>/handoff
// and writes the decoded data.handoff to out.
func TestRunHandoffDump_PrintsHandoff(t *testing.T) {
	sock := newHandoffTestSocket(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v0/projects/p1/repos/r1/workspaces/w1/chats/chat-1/handoff", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data":    map[string]any{"handoff": "X"},
		})
	})

	var out bytes.Buffer
	err := runHandoffDump("chat-1", "p1", "r1", "w1", "unix://"+sock, &out)
	require.NoError(t, err)
	require.Equal(t, "X", out.String())
}

// TestRunHandoffDump_DaemonError proves a {success:false,error:...} envelope
// surfaces as a Go error rather than being printed as if it were the handoff.
func TestRunHandoffDump_DaemonError(t *testing.T) {
	sock := newHandoffTestSocket(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "agent chat not found",
		})
	})

	var out bytes.Buffer
	err := runHandoffDump("missing", "p1", "r1", "w1", "unix://"+sock, &out)
	require.Error(t, err)
	require.Contains(t, err.Error(), "agent chat not found")
	require.Empty(t, out.String())
}
