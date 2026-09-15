package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
	require.Equal(t, "/v0/projects/p1/repos/r1/workspaces/w1/chats/hooks", gotPath)
	require.Equal(t, "seg-42", got["segment_id"])
	require.Equal(t, "claude", got["provider"])
	require.Equal(t, "turn_stop", got["event"])
	require.Equal(t, `{"session_id":"abc"}`, got["payload_raw"])
	require.NotEmpty(t, got["delivery_id"])
}

func TestNewHookCmd_HomeFlagOverridesEnv(t *testing.T) {
	t.Setenv("CROWBAR_HOME", "/wrong/home")
	cmd := newHookCmd()
	cmd.SetArgs([]string{"session_start", "--home", "/tmp/right-home", "--payload", "{}"})
	// The command swallows all errors (must never break the vendor CLI), so
	// Execute always returns nil; what we assert is the env var it left behind.
	_ = cmd.Execute()
	if got := os.Getenv("CROWBAR_HOME"); got != "/tmp/right-home" {
		t.Fatalf("CROWBAR_HOME = %q, want %q", got, "/tmp/right-home")
	}
}

// TestRunHook_RetriesTransientFailureWithSameDeliveryID covers the race
// barriers_test.go documents: a hook can fire before the runner row it
// targets is queryable yet, failing once for reasons that clear a moment
// later. runHook must retry in-process, a few times, reusing one delivery id
// — not drop the event, and not hand it to any on-disk queue.
func TestRunHook_RetriesTransientFailureWithSameDeliveryID(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())
	sock := filepath.Join(shortSocketDir(t), "h.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer ln.Close()

	var mu sync.Mutex
	var deliveries []string
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		mu.Lock()
		deliveries = append(deliveries, got["delivery_id"].(string))
		attempt := len(deliveries)
		mu.Unlock()
		if attempt < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	err = runHook(hookRun{
		Event: "user_prompt", Segment: "seg-1", Provider: "codex",
		Project: "p", Repo: "r", Workspace: "w",
		Payload: []byte(`{"prompt":"keep me"}`), Host: "unix://" + sock, Out: io.Discard,
	})
	require.NoError(t, err, "a retry that eventually succeeds must not surface as a hook error")

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, deliveries, 2)
	require.Equal(t, deliveries[0], deliveries[1], "every retry must reuse the original delivery id")
}

// TestRunHook_GivesUpAfterExhaustingRetries asserts the OTHER half of "never
// blocks the CLI": once the bounded retry is exhausted, runHook returns
// (surfacing an error internally, printed to stderr by the caller) rather
// than hanging or persisting the event anywhere for something else to retry
// later.
func TestRunHook_GivesUpAfterExhaustingRetries(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())
	sock := filepath.Join(shortSocketDir(t), "h.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer ln.Close()

	var attempts atomic.Int64
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	err = runHook(hookRun{
		Event: "user_prompt", Segment: "seg-1", Provider: "codex",
		Project: "p", Repo: "r", Workspace: "w",
		Payload: []byte(`{"prompt":"drop me"}`), Host: "unix://" + sock, Out: io.Discard,
	})
	require.Error(t, err)
	require.Equal(t, int64(hookDeliveryAttempts), attempts.Load())
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
