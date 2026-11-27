//go:build unix

package descriptorcheck

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

type term struct {
	cmd  *exec.Cmd
	ptmx *os.File

	mu  sync.Mutex
	out bytes.Buffer

	done    chan struct{}
	waitErr error
}

const maxTermOutput = 1 << 20

const drainBound = 2 * time.Second

var (
	cursorQuery = []byte("\x1b[6n")
	deviceQuery = []byte("\x1b[c")
)

func startTerm(ctx context.Context, argv, env []string, cwd string) (*term, error) {
	if len(argv) == 0 {
		return nil, errors.New("descriptorcheck: empty argv")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // the argv is the descriptor under test, run on purpose
	cmd.Env = append(append([]string{}, env...), "TERM=xterm-256color")
	cmd.Dir = cwd
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		return nil, fmt.Errorf("descriptorcheck: start %s: %w", argv[0], err)
	}
	t := &term{cmd: cmd, ptmx: ptmx, done: make(chan struct{})}
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		t.pump()
	}()
	go func() {
		err := cmd.Wait()
		timer := time.NewTimer(drainBound)
		defer timer.Stop()
		select {
		case <-drained:
		case <-timer.C:
		}
		t.waitErr = err
		close(t.done)
	}()
	return t, nil
}

func (t *term) pump() {
	buf := make([]byte, 32*1024)
	for {
		n, err := t.ptmx.Read(buf)
		if n > 0 {
			t.take(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func (t *term) take(chunk []byte) {
	if bytes.Contains(chunk, cursorQuery) {
		t.reply("\x1b[1;1R")
	}
	if bytes.Contains(chunk, deviceQuery) {
		t.reply("\x1b[?1;2c")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.out.Len() < maxTermOutput {
		t.out.Write(chunk)
	}
}

func (t *term) reply(answer string) {
	if _, err := t.ptmx.Write([]byte(answer)); err != nil {
		slog.Warn("descriptorcheck: answer a terminal query", "err", err)
	}
}

var (
	csiRE   = regexp.MustCompile(`\x1b\[[0-9;?<>=]*[ -/]*[@-~]`)
	oscRE   = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	escRE   = regexp.MustCompile(`\x1b[()*+][A-Za-z0-9]|\x1b[=>78DEHMc]`)
	spaceRE = regexp.MustCompile(`[ \t\r]+`)
)

func (t *term) text() string {
	t.mu.Lock()
	raw := t.out.String()
	t.mu.Unlock()
	s := oscRE.ReplaceAllString(raw, "")
	s = csiRE.ReplaceAllString(s, " ")
	s = escRE.ReplaceAllString(s, "")
	return spaceRE.ReplaceAllString(s, " ")
}

func (t *term) send(keys string) error {
	if _, err := t.ptmx.Write([]byte(keys)); err != nil {
		return fmt.Errorf("descriptorcheck: type into the terminal: %w", err)
	}
	return nil
}

func (t *term) exited(d time.Duration) (bool, int) {
	select {
	case <-t.done:
		return true, exitCode(t.waitErr)
	case <-time.After(d):
		return false, 0
	}
}

func (t *term) close() {
	if err := killGroup(t.cmd); err != nil {
		slog.Warn("descriptorcheck: kill the CLI", "err", err)
	}
	select {
	case <-t.done:
	case <-time.After(5 * time.Second):
	}
	if err := t.ptmx.Close(); err != nil {
		slog.Warn("descriptorcheck: close the terminal", "err", err)
	}
}

func killGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("kill process group: %w", err)
	}
	return nil
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	if err != nil {
		return -1
	}
	return 0
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
