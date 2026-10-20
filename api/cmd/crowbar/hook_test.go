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
	"time"

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

// serveHooks serves handler on a fresh unix socket for the rest of the test and
// returns the host a hook relay dials.
func serveHooks(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	sock := filepath.Join(shortSocketDir(t), "h.sock")
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	t.Cleanup(func() {
		require.NoError(t, srv.Close())
		require.ErrorIs(t, <-served, http.ErrServerClosed)
	})
	return "unix://" + sock
}

func TestRunHook_ForwardsSegmentProviderAndRawPayload(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())

	var mu sync.Mutex
	var gotPath string
	var got map[string]any
	host := serveHooks(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPath = r.URL.RequestURI()
		_ = json.NewDecoder(r.Body).Decode(&got)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	})

	err := runHook(hookRun{
		Event: "turn_stop", Segment: "seg-42", Provider: "claude",
		Project: "p1", Repo: "r1", Workspace: "w1",
		Payload: []byte(`{"session_id":"abc"}`), Host: host, Out: io.Discard,
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "/v0/projects/p1/repos/r1/chats/hooks", gotPath)
	require.Equal(t, "seg-42", got["segment_id"])
	require.Equal(t, "claude", got["provider"])
	require.Equal(t, "turn_stop", got["event"])
	require.Equal(t, `{"session_id":"abc"}`, got["payload_raw"])
}

func TestRunHook_PrintsTheReplyTheDaemonAnswersWith(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())
	host := serveHooks(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"data":{"reply":"{\"decision\":\"block\"}"}}`)
	})

	var out strings.Builder
	err := runHook(hookRun{
		Event: "turn_stop", Segment: "seg-42", Provider: "claude",
		Project: "p1", Repo: "r1", Workspace: "w1",
		Payload: []byte(`{}`), Host: host, Out: &out,
	})

	require.NoError(t, err)
	require.JSONEq(t, `{"decision":"block"}`, strings.TrimSpace(out.String()))
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

// TestRunHook_RetriesATransientFailure: a daemon that fails for a reason that
// clears a moment later gets the hook again, in-process, a few times — the
// event is not dropped, and not handed to any on-disk queue.
func TestRunHook_RetriesATransientFailure(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())

	var attempts atomic.Int64
	host := serveHooks(t, func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})

	err := runHook(hookRun{
		Event: "user_prompt", Segment: "seg-1", Provider: "codex",
		Project: "p", Repo: "r", Workspace: "w",
		Payload: []byte(`{"prompt":"keep me"}`), Host: host, Out: io.Discard,
	})
	require.NoError(t, err, "a retry that eventually succeeds must not surface as a hook error")
	require.Equal(t, int64(2), attempts.Load())
}

// TestRunHook_ARefusalIsNotRetried: a 4xx is the daemon's verdict on the hook
// (an unknown runner, an unmappable payload), recorded once; sending it again
// would only record it again.
func TestRunHook_ARefusalIsNotRetried(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())

	var attempts atomic.Int64
	host := serveHooks(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusUnprocessableEntity)
	})

	err := runHook(hookRun{
		Event: "user_prompt", Segment: "seg-1", Provider: "codex",
		Project: "p", Repo: "r", Workspace: "w",
		Payload: []byte(`{}`), Host: host, Out: io.Discard,
	})
	require.Error(t, err)
	require.Equal(t, int64(1), attempts.Load())
}

// TestRunHook_GivesUpAfterExhaustingRetries asserts the OTHER half of "never
// blocks the CLI": once the bounded retry is exhausted, runHook returns
// (surfacing an error internally, printed to stderr by the caller) rather
// than hanging or persisting the event anywhere for something else to retry
// later.
func TestRunHook_GivesUpAfterExhaustingRetries(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())

	var attempts atomic.Int64
	host := serveHooks(t, func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	err := runHook(hookRun{
		Event: "user_prompt", Segment: "seg-1", Provider: "codex",
		Project: "p", Repo: "r", Workspace: "w",
		Payload: []byte(`{"prompt":"drop me"}`), Host: host, Out: io.Discard,
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
