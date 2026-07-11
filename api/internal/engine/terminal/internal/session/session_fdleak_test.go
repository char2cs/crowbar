package session

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// openFDs counts this process's open file descriptors. Both macOS and Linux expose
// them as /dev/fd entries. Names only: os.ReadDir would stat every entry, and on
// macOS the entry for its own directory descriptor fails that stat. The descriptor
// this function opens is present in every sample, so successive counts stay directly
// comparable.
func openFDs(
	t *testing.T,
) int {
	t.Helper()
	dir, err := os.Open("/dev/fd")
	require.NoError(t, err)
	defer dir.Close()
	names, err := dir.Readdirnames(-1)
	require.NoError(t, err)
	return len(names)
}

// runSessionToNaturalExit spawns a shell, tells it to exit, and blocks on the
// session's own termination signal (Done, closed at the end of shutdown) — never on
// a clock. It returns the PTY master so the caller can keep it reachable.
func runSessionToNaturalExit(
	t *testing.T,
	id string,
) *os.File {
	t.Helper()
	s, err := New(id, "/bin/sh", t.TempDir(), "", os.Environ(), 80, 24, 0)
	require.NoError(t, err)

	s.mu.Lock()
	ptmx := s.ptmx
	s.mu.Unlock()
	require.NotNil(t, ptmx, "a freshly spawned session must hold a live PTY master")

	require.NoError(t, s.Write([]byte("exit\n")))
	<-s.Done()
	return ptmx
}

// A shell that exits on its own (exit / Ctrl-D / process death) tears the session
// down through pump's deferred shutdown, not through Kill. shutdown must therefore
// close the PTY master itself: dropping the *os.File without closing it strands the
// descriptor, and a later Kill cannot recover it because ptmx is already nil.
//
// Every master is pinned for the whole test on purpose. An unreachable os.File has
// its descriptor closed by the runtime finalizer at the next GC, which would mask
// the leak here — while in production it is the leak: an idle daemon allocates
// nothing, so the descriptor is held until a GC that may never come. Pinning takes
// the finalizer out of the picture, leaving an explicit Close as the only thing that
// can return the descriptor.
func TestSession_NaturalExitClosesPTYMaster(t *testing.T) {
	// The first spawn also allocates the runtime poller's own descriptor; take the
	// baseline after it so that one-off is not read as a leak.
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
