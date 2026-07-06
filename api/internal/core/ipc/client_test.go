package ipc_test

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
	"github.com/stretchr/testify/require"
)

func TestClient_PostJSON_OverUnixSocket(t *testing.T) {
	// Use a short temp directory to stay under macOS's 104-byte sun_path limit
	tmpDir := filepath.Join("/tmp", "cs.sock")
	sock := filepath.Join(tmpDir, "t.sock")
	defer os.RemoveAll(tmpDir)
	_ = os.MkdirAll(tmpDir, 0755)
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer ln.Close()

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v0/agent/hooks", r.URL.Path)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"success":true}`))
	})}
	go srv.Serve(ln)
	defer srv.Close()

	c, err := ipc.NewClient("unix://" + sock)
	require.NoError(t, err)
	status, body, err := c.PostJSON(context.Background(), "/v0/agent/hooks", map[string]string{"event": "session_start"})
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, status)
	require.Contains(t, string(body), "success")
	_ = os.Remove(sock)
}
