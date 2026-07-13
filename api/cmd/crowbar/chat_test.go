package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunChatRename_PostsTitleWithAgentSource(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "chat")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)
	sock := filepath.Join(tmpDir, "c.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer ln.Close()

	var mu sync.Mutex
	var gotPath string
	var gotBody map[string]any
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPath = r.URL.RequestURI()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	err = runChatRename("seg-42", "Fix Auth Flow", "p1", "r1", "w1", "unix://"+sock)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "/v0/projects/p1/repos/r1/workspaces/w1/agent/runners/seg-42/rename?source=agent", gotPath)
	require.Equal(t, "Fix Auth Flow", gotBody["title"])
}
