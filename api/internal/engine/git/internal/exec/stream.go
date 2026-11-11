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

// stderrCaptureLimit bounds retained stderr: git on a corrupt repo emits megabytes, and a failure
// path proportional to output undoes what streaming bought. Far above any real diagnostic.
const stderrCaptureLimit = 64 << 10

// GitStream runs git in dir and streams stdout instead of buffering tens of megabytes. Close (required)
// ends the subprocess early and reaps it; wait reports a non-zero exit as *GitError, idempotently.
// No lock retry: a re-run would duplicate bytes already consumed, and streamed commands are read-only.
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

// startStream is safe to retry whole when the resolver recovers a vanished binary: only StdoutPipe
// and Start can fail and both are pre-exec, so an error means no process ran and nothing was read.
func startStream(
	ctx context.Context,
	bin string,
	dir string,
	args []string,
) (*gitStream, error) {
	sctx, cancel := context.WithCancel(ctx)
	cmd := command(sctx, bin, dir, args)

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

// Close kills the subprocess unless EOF was reached, closes the read end so git cannot wedge on a
// full pipe, and reaps. The EOF check stops a fully-read stream reporting the cancellation as an error.
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

// finish reaps once and records the perf sample here, so it covers the whole time the consumer
// spent reading, not just the fork.
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
	// A signal leaves no git-produced stderr, and a watchdog failure can even
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
