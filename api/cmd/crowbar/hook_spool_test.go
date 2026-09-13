package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
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

	client, err := ipc.NewClient("unix://" + sock)
	require.NoError(t, err)
	for i := 0; i < maxDeliveryAttempts+1; i++ {
		_ = drainHookSpool(context.Background(), client)
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

// TestRegression_DrainHookSpoolReusesOneConnectionAcrossManyEnvelopesAndTicks
// guards the resource leak that killed a live production daemon twice in one
// day: deliverHookEnvelope used to call ipc.NewClient — building a brand-new,
// never-expiring http.Transport — on EVERY delivery attempt. drainHookSpoolLoop
// ticks once a second for the daemon's entire life, sweeping every file still
// in the spool on every tick; that leaked one goroutine and one socket per
// attempt, forever. A crashed daemon's own goroutine dump was found holding
// 2480 such goroutines, all idle for hours, when its watchdog SIGKILLed it.
//
// The fix builds ONE *ipc.Client per caller (drainHookSpoolLoop for the
// daemon's whole lifetime, one hook CLI invocation for its own) and threads it
// through every delivery. This asserts the observable effect: many envelopes
// delivered across many separate drain passes, sharing one client, open at
// most one physical connection — proven by counting the stub daemon's own
// Accept() calls rather than timing anything.
func TestRegression_DrainHookSpoolReusesOneConnectionAcrossManyEnvelopesAndTicks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CROWBAR_HOME", home)
	sock := filepath.Join(shortSocketDir(t), "h.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer ln.Close()
	counting := &countingListener{Listener: ln}

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})}
	go srv.Serve(counting)
	defer srv.Close()

	const envelopes = 5
	for i := 0; i < envelopes; i++ {
		writeSpooledEnvelope(t, home, fmt.Sprintf("%020d", i), hookEnvelope{
			DeliveryID: fmt.Sprintf("d%d", i), Project: "p", Workspace: "w",
			Event: "turn_stop", CreatedAt: "2026-09-10T00:00:00Z",
		})
	}

	client, err := ipc.NewClient("unix://" + sock)
	require.NoError(t, err)

	// Three separate drain passes, exactly like three drainHookSpoolLoop
	// ticks: the leak was per call to ipc.NewClient inside deliverHookEnvelope,
	// not per drain pass, so a regression here must span passes, not just
	// envelopes within one pass.
	for i := 0; i < 3; i++ {
		require.NoError(t, drainHookSpool(context.Background(), client))
	}

	require.LessOrEqual(t, counting.accepted.Load(), int64(1),
		"one shared *ipc.Client delivering 5 envelopes across 3 drain passes must reuse its "+
			"pooled connection — each additional Accept() here is a leaked goroutine+socket in "+
			"the real daemon, one per delivery attempt, forever")
}

// countingListener counts every accepted connection so a test can assert on
// connection reuse without touching timing.
type countingListener struct {
	net.Listener
	accepted atomic.Int64
}

func (c *countingListener) Accept() (net.Conn, error) {
	conn, err := c.Listener.Accept()
	if err == nil {
		c.accepted.Add(1)
	}
	return conn, err
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
