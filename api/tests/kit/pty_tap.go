//go:build integration

package kit

import (
	"context"
	"io"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
)

type Signal struct {
	mu sync.Mutex
	ch chan struct{}
}

func NewSignal() *Signal {
	return &Signal{ch: make(chan struct{})}
}

func (s *Signal) Wait() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ch
}

func (s *Signal) Fire() {
	s.mu.Lock()
	defer s.mu.Unlock()
	close(s.ch)
	s.ch = make(chan struct{})
}

func Await[T any](
	t *testing.T,
	ctx context.Context,
	what string,
	pred func() (T, bool),
	sigs ...*Signal,
) T {
	t.Helper()
	var last T
	for {
		cases := make([]reflect.SelectCase, 0, len(sigs)+1)
		for _, s := range sigs {
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(s.Wait()),
			})
		}
		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(ctx.Done()),
		})

		v, ok := pred()
		last = v
		if ok {
			return v
		}

		if chosen, _, _ := reflect.Select(cases); chosen == len(cases)-1 {
			t.Fatalf("kit.Await: %s: backstop expired before the condition held; last value observed: %+v", what, last)
			return last
		}
	}
}

var ansiRe = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b\[[0-?]*[ -/]*[@-~]|\x1b[@-Z\\-_]`)

func squeeze(s string) string {
	stripped := ansiRe.ReplaceAllString(s, "")
	var b strings.Builder
	b.Grow(len(stripped))
	for _, r := range stripped {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func Readable(
	raw string,
) string {
	stripped := ansiRe.ReplaceAllString(raw, "")
	stripped = strings.ReplaceAll(stripped, "\r", "\n")
	var keep []string
	for _, line := range strings.Split(stripped, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			keep = append(keep, trimmed)
		}
	}
	return strings.Join(keep, "\n")
}

type PTYTap struct {
	mu        sync.Mutex
	buf       []byte
	sig       *Signal
	closed    chan struct{}
	closeOnce sync.Once
}

func AttachPTY(
	t *testing.T,
	term PTYAttacher,
	sessionID string,
) *PTYTap {
	t.Helper()
	tap := &PTYTap{sig: NewSignal(), closed: make(chan struct{})}
	go func() { _ = term.Attach(context.Background(), sessionID, tap) }()
	t.Cleanup(func() { _ = tap.Close() })
	return tap
}

type PTYAttacher interface {
	Attach(ctx context.Context, sessionID string, conn terminal.WSConn) error
}

var _ terminal.WSConn = (*PTYTap)(nil)

func (p *PTYTap) WriteMessage(
	_ int,
	msg []byte,
) error {
	data, _, ok := ParseTerminalFrame(msg)
	if !ok || len(data) == 0 {
		return nil
	}
	p.mu.Lock()
	p.buf = append(p.buf, data...)
	p.mu.Unlock()
	p.sig.Fire()
	return nil
}

func (p *PTYTap) SetWriteDeadline(time.Time) error { return nil }

func (p *PTYTap) ReadMessage() (int, []byte, error) {
	<-p.closed
	return 0, nil, io.EOF
}

func (p *PTYTap) Close() error {
	p.closeOnce.Do(func() { close(p.closed) })
	return nil
}

func (p *PTYTap) Screen() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return string(p.buf)
}

func (p *PTYTap) Mark() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.buf)
}

func (p *PTYTap) ScreenSince(
	mark int,
) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if mark > len(p.buf) {
		return ""
	}
	return string(p.buf[mark:])
}

func (p *PTYTap) WaitForScreenSince(
	t *testing.T,
	ctx context.Context,
	mark int,
	needle string,
) {
	t.Helper()
	Await(t, ctx, "PTY to paint "+needle, func() (bool, bool) {
		ok := strings.Contains(squeeze(p.ScreenSince(mark)), squeeze(needle))
		return ok, ok
	}, p.sig)
}

func (p *PTYTap) Signal() *Signal { return p.sig }

func (p *PTYTap) Contains(
	needle string,
) bool {
	return strings.Contains(squeeze(p.Screen()), squeeze(needle))
}

func (p *PTYTap) ContainsAny(
	needles ...string,
) (string, bool) {
	hay := squeeze(p.Screen())
	for _, n := range needles {
		if strings.Contains(hay, squeeze(n)) {
			return n, true
		}
	}
	return "", false
}

func (p *PTYTap) WaitForScreen(
	t *testing.T,
	ctx context.Context,
	needle string,
) string {
	t.Helper()
	Await(t, ctx, "PTY to paint "+needle, func() (bool, bool) {
		ok := p.Contains(needle)
		return ok, ok
	}, p.sig)
	return p.Screen()
}

func ScreenContains(
	screen string,
	needle string,
) bool {
	return strings.Contains(squeeze(screen), squeeze(needle))
}

func EchoNeedle(
	text string,
) string {
	const tail = 24
	squeezed := squeeze(text)
	if len(squeezed) <= tail {
		return squeezed
	}
	return squeezed[len(squeezed)-tail:]
}
