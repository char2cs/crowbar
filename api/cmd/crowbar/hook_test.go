package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// shortSocketDir keeps a test socket under the ~104-byte sun_path limit that
// bind(2) enforces on macOS. t.TempDir() embeds the full test name and blows
// past it, failing with a bare "bind: invalid argument".
func shortSocketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "cbhook")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestRunHook_ForwardsSegmentProviderAndRawPayload(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())
	sock := filepath.Join(shortSocketDir(t), "h.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer ln.Close()

	var mu sync.Mutex
	var gotPath string
	var got map[string]any
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPath = r.URL.RequestURI()
		_ = json.NewDecoder(r.Body).Decode(&got)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	err = runHook(hookRun{
		Event: "turn_stop", Segment: "seg-42", Provider: "claude",
		Project: "p1", Repo: "r1", Workspace: "w1",
		Payload: []byte(`{"session_id":"abc"}`), Host: "unix://" + sock, Out: io.Discard,
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "/v0/projects/p1/repos/r1/chats/hooks", gotPath)
	require.Equal(t, "seg-42", got["segment_id"])
	require.Equal(t, "claude", got["provider"])
	require.Equal(t, "turn_stop", got["event"])
	require.Equal(t, `{"session_id":"abc"}`, got["payload_raw"])
	require.NotEmpty(t, got["delivery_id"])
	entries, err := os.ReadDir(hookSpoolDir())
	require.NoError(t, err)
	for _, entry := range entries {
		require.NotEqual(t, ".json", filepath.Ext(entry.Name()), "acknowledged hook must leave no envelope")
	}
}

func TestRunHook_Non2xxRemainsSpooledAndRetriesSameDeliveryID(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())
	sock := filepath.Join(shortSocketDir(t), "h.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer ln.Close()

	var mu sync.Mutex
	var deliveries []string
	status := http.StatusServiceUnavailable
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		mu.Lock()
		deliveries = append(deliveries, got["delivery_id"].(string))
		current := status
		mu.Unlock()
		w.WriteHeader(current)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	host := "unix://" + sock
	err = runHook(hookRun{
		Event: "user_prompt", Segment: "seg-1", Provider: "codex",
		Project: "p", Repo: "r", Workspace: "w",
		Payload: []byte(`{"prompt":"keep me"}`), Host: host, Out: io.Discard,
	})
	require.Error(t, err)
	files, err := filepath.Glob(filepath.Join(hookSpoolDir(), "*.json"))
	require.NoError(t, err)
	require.Len(t, files, 1, "non-2xx must retain the complete durable envelope")

	mu.Lock()
	status = http.StatusAccepted
	firstID := deliveries[0]
	mu.Unlock()
	require.NoError(t, drainHookSpool(context.Background(), host))
	files, err = filepath.Glob(filepath.Join(hookSpoolDir(), "*.json"))
	require.NoError(t, err)
	require.Empty(t, files)
	mu.Lock()
	require.Len(t, deliveries, 2)
	require.Equal(t, firstID, deliveries[1], "retry must reuse the original delivery id")
	mu.Unlock()
}

func TestResolvePayload_Precedence(t *testing.T) {
	f := filepath.Join(t.TempDir(), "p.json")
	require.NoError(t, os.WriteFile(f, []byte("FROMFILE"), 0o644))

	inline, err := resolvePayload("INLINE", f, strings.NewReader("FROMSTDIN"))
	require.NoError(t, err)
	require.Equal(t, "INLINE", string(inline))

	fromFile, err := resolvePayload("", f, strings.NewReader("FROMSTDIN"))
	require.NoError(t, err)
	require.Equal(t, "FROMFILE", string(fromFile))

	fromStdin, err := resolvePayload("", "", strings.NewReader("FROMSTDIN"))
	require.NoError(t, err)
	require.Equal(t, "FROMSTDIN", string(fromStdin))
}

func TestResolvePayload_RejectsOversizeWithoutSilentTruncation(t *testing.T) {
	_, err := resolvePayload("", "", strings.NewReader(strings.Repeat("x", maxHookPayloadBytes+1)))
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds")
}
