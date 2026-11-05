//go:build unix

package runner

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Serve is ready when it answers the handshake: one that is alive but silent
// has given its runner neither a connection nor a PTY.
func TestStartAPIConn_ServeIsReadyOnlyOnceItAnswers(t *testing.T) {
	h := helperServeSpawn(t, "answer", false)
	gate := filepath.Join(t.TempDir(), "gate")
	require.NoError(t, syscall.Mkfifo(gate, 0o600))
	t.Setenv(helperServeGate, gate)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	spawned := make(chan error, 1)
	go func() {
		_, err := h.rs.spawnRunner(ctx, h.chatID, "ws-1", "carrier-test", spawnInput{runnerID: "runner-1"}, false)
		spawned <- err
	}()
	opened := make(chan *os.File, 1)
	go func() {
		f, err := os.OpenFile(gate, os.O_WRONLY, 0) // returns once serve opens its end
		if assert.NoError(t, err) {
			opened <- f
		}
	}()
	select {
	case release := <-opened:
		assert.False(t, h.rs.HasLiveAPIConnection("runner-1"), "a silent serve was taken as ready")
		assert.Zero(t, h.term.callCount(), "a silent serve was given up for a PTY")
		require.NoError(t, release.Close())
	case err := <-spawned:
		t.Fatalf("the spawn went on before its serve answered: %v", err)
	case <-ctx.Done():
		t.Fatal("serve never started")
	}

	require.NoError(t, <-spawned)
	assert.True(t, h.rs.HasLiveAPIConnection("runner-1"))
	assert.Empty(t, h.term.created, "the answered connection is the runner")
}
