package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDrainHookSpool_SkipsPastAPermanentlyFailingEnvelope writes two spooled
// envelopes: one scoped to a project the stub daemon 404s on every request
// (simulating a deleted project), one scoped to a project it accepts. Before
// this fix, the failing envelope — first in FIFO order — aborted the whole
// pass and the second envelope was NEVER attempted, no matter how many times
// the loop ticked.
func TestDrainHookSpool_SkipsPastAPermanentlyFailingEnvelope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CROWBAR_HOME", home)
	sock := filepath.Join(shortSocketDir(t), "h.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer ln.Close()

	var mu sync.Mutex
	var delivered []string
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/projects/dead-project/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		mu.Lock()
		delivered = append(delivered, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	writeSpooledEnvelope(t, home, "00000000000000000001", hookEnvelope{
		DeliveryID: "dead", Project: "dead-project", Workspace: "w1",
		Event: "turn_stop", CreatedAt: "2026-09-10T00:00:00Z",
	})
	writeSpooledEnvelope(t, home, "00000000000000000002", hookEnvelope{
		DeliveryID: "live", Project: "live-project", Workspace: "w1",
		Event: "turn_stop", CreatedAt: "2026-09-10T00:00:01Z",
	})

	for i := 0; i < maxDeliveryAttempts+1; i++ {
		_ = drainHookSpool(context.Background(), "unix://"+sock)
	}

	mu.Lock()
	gotDelivered := append([]string(nil), delivered...)
	mu.Unlock()
	require.Contains(t, gotDelivered, "/v0/projects/live-project/home/chats/hooks",
		"the live envelope must be delivered even though the dead one, queued first, never succeeds")

	deadLetterDir := filepath.Join(hookSpoolDir(), "dead-letter")
	entries, err := os.ReadDir(deadLetterDir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "the permanently-failing envelope must be moved to dead-letter, not retried forever")
}

// writeSpooledEnvelope writes envelope straight into home's hook-spool with
// the given sort-order prefix, bypassing persistHookEnvelope's own filename
// scheme so the test controls FIFO order directly.
func writeSpooledEnvelope(t *testing.T, home, prefix string, envelope hookEnvelope) {
	t.Helper()
	dir := filepath.Join(home, "hook-spool")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	data, err := json.Marshal(envelope)
	require.NoError(t, err)
	name := prefix + "-" + envelope.DeliveryID + ".json"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o600))
}
