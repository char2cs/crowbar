package exec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/char2cs/crowbar/api/internal/perf"
)

// stderrCaptureLimit bounds what a streamed invocation retains of stderr. git
// on a corrupt repo emits it by the megabyte, and a stream whose failure path
// is proportional to the output size gives back exactly what streaming bought.
// The cap is far above any real git diagnostic, so a truncated capture only
// ever happens where the message was already unreadable.
const stderrCaptureLimit = 64 << 10

// GitStream runs a git command in dir and hands its stdout back as the
// subprocess produces it, where Git buffers the whole of it into a string
// first. A branch-wide `diff`/`log -p` runs to tens of megabytes, so the
// buffering path costs that much resident memory for the duration of every
// concurrent call — output its consumers walk a line at a time and discard.
//
// The caller must Close the reader. Closing before EOF kills the subprocess, so
// a consumer that has seen enough — a paginated patch, a search that found its
// match — stops paying for the rest; the returned wait then describes that kill
// rather than the command's own verdict.
//
// wait blocks until the subprocess exits and reports a non-zero exit as a
// *GitError, the shape RequireSuccess produces. It may be called any number of
// times and reports the same result each time. Close reaps the subprocess too,
// so a caller that abandons a stream without calling wait leaves nothing behind.
//
// Streams deliberately do not go through runWithLockRetry. That helper re-runs
// a whole invocation after a contended index.lock, which cannot work once the
// first attempt's bytes have been handed to a consumer — the retry would
// duplicate the prefix the consumer already read. It costs nothing here: the
// commands worth streaming are read-only inspections (diff, log, show, grep)
// that never take the index lock, and GIT_OPTIONAL_LOCKS=0 keeps it that way.
func GitStream(
	ctx context.Context,
	dir string,
	args ...string,
) (io.ReadCloser, func() error, error) {
	s, err := startStream(ctx, gitBin(), dir, args)
	if err != nil && recoverGit() {
		s, err = startStream(ctx, gitBin(), dir, args)
	}
	if err != nil {
		return nil, nil, err
	}
	return s, s.wait, nil
}

// startStream is retried as a whole when the resolver recovers a vanished
// binary. Nothing survives a failed attempt to retry around: the subprocess did
// not start, so no stdout byte can have reached a consumer, and the abandoned
// context is cancelled here rather than by the caller.
//
// Its error is a sound never-started signal in a way the buffered path's is not,
// and needs no equivalent of that path's cmd.Process check. Only two things can
// fail here, StdoutPipe and Start, and both are strictly pre-exec: os/exec's
// Start returns nil the moment StartProcess succeeds, and every step it takes
// afterwards is infallible. A non-nil error therefore means no process exists,
// so the retry cannot re-run a command that already did its work.
func startStream(
	ctx context.Context,
	bin string,
	dir string,
	args []string,
) (*gitStream, error) {
	sctx, cancel := context.WithCancel(ctx)
	//nolint:gosec // G204: running git with caller-supplied args is the purpose of this package.
	cmd := exec.CommandContext(sctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = gitEnv(nil)
	cmd.WaitDelay = waitDelay

	s := &gitStream{cmd: cmd, cancel: cancel, op: subcommandName(args)}
	cmd.Stderr = &s.stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("git %s: stdout pipe: %w", s.op, err)
	}
	s.stdout = stdout

	s.started = time.Now()
	if startErr := cmd.Start(); startErr != nil {
		cancel()
		return nil, fmt.Errorf("git %s: %w", s.op, startErr)
	}
	return s, nil
}

type gitStream struct {
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	op      string
	started time.Time
	stdout  io.ReadCloser
	stderr  limitedBuffer

	drained atomic.Bool
	once    sync.Once
	waitErr error
}

func (s *gitStream) Read(
	p []byte,
) (int, error) {
	n, err := s.stdout.Read(p)
	if errors.Is(err, io.EOF) {
		s.drained.Store(true)
	}
	return n, err
}

// Close kills the subprocess unless the reader already reached EOF, closes the
// read end of the pipe, and reaps. Closing the read end is what keeps an
// early-closing consumer from wedging git on a full pipe buffer: a write with
// no reader fails rather than blocking forever. The EOF check matters because
// cancelling a context whose command already succeeded makes os/exec report the
// cancellation in place of the exit status, which would turn every
// fully-consumed stream into a spurious error.
func (s *gitStream) Close() error {
	if !s.drained.Load() {
		s.cancel()
	}
	err := s.stdout.Close()
	s.finish()
	if errors.Is(err, os.ErrClosed) {
		return nil
	}
	return err
}

func (s *gitStream) wait() error {
	s.finish()
	return s.exitError()
}

// finish reaps exactly once and records the sample the perf ring expects for
// every git subprocess. Recording here rather than at GitStream's return is
// what makes the sample cover the whole streamed duration — the interesting
// part of a stream is how long the consumer spent reading it, not how long the
// fork took.
func (s *gitStream) finish() {
	s.once.Do(func() {
		s.waitErr = s.cmd.Wait()
		perf.Record("git."+s.op, time.Since(s.started))
	})
}

func (s *gitStream) exitError() error {
	if s.waitErr == nil {
		return nil
	}
	r := Result{Stderr: s.stderr.String(), ExitCode: exitCode(s.cmd, s.waitErr)}
	// A kill leaves no git-produced stderr, and a watchdog failure can even
	// leave a zero exit status on a command that never ran to completion.
	// Neither may be reported as success.
	if r.ExitCode == 0 {
		r.ExitCode = 1
	}
	if r.Stderr == "" {
		r.Stderr = s.waitErr.Error()
	}
	return RequireSuccess(s.op, r)
}

type limitedBuffer struct {
	b []byte
}

func (l *limitedBuffer) Write(
	p []byte,
) (int, error) {
	room := min(stderrCaptureLimit-len(l.b), len(p))
	if room > 0 {
		l.b = append(l.b, p[:room]...)
	}
	return len(p), nil
}

func (l *limitedBuffer) String() string {
	return string(l.b)
}
