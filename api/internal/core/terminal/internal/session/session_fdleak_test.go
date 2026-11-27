package session

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func openFDs(
	t *testing.T,
) int {
	t.Helper()
	dir, err := os.Open("/dev/fd")
	require.NoError(t, err)
	defer func() { _ = dir.Close() }()
	names, err := dir.Readdirnames(-1)
	require.NoError(t, err)
	return len(names)
}

func runSessionToNaturalExit(
	t *testing.T,
	id string,
) *os.File {
	t.Helper()
	s, err := New(t.Context(), id, "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 0)
	require.NoError(t, err)

	s.mu.Lock()
	ptmx := s.ptmx
	s.mu.Unlock()
	require.NotNil(t, ptmx, "a freshly spawned session must hold a live PTY master")

	require.NoError(t, s.Write([]byte("exit\n")))
	<-s.Done()
	return ptmx
}

func TestSession_NaturalExitClosesPTYMaster(t *testing.T) {
	pinned := []*os.File{runSessionToNaturalExit(t, "sid-fd-warmup")}
	baseline := openFDs(t)

	for _, id := range []string{"sid-fd-1", "sid-fd-2", "sid-fd-3"} {
		pinned = append(pinned, runSessionToNaturalExit(t, id))
	}
	after := openFDs(t)
	runtime.KeepAlive(pinned)

	require.Equal(t, baseline, after,
		"the PTY master fd of a naturally-exited session must be closed by shutdown, not left to the finalizer")
}
