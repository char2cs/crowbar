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

// Signal is a broadcast edge: every Wait() is released by the next Fire(). Fire swaps the channel so a
// caller that takes it BEFORE checking its predicate cannot lose an event landing between check and park.
type Signal struct {
	mu sync.Mutex
	ch chan struct{}
}

// NewSignal returns a Signal with no waiters.
func NewSignal() *Signal {
	return &Signal{ch: make(chan struct{})}
}

// Wait returns a channel closed by the next Fire. Take it BEFORE evaluating the
// condition you intend to park on (see the type comment).
func (s *Signal) Wait() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ch
}

// Fire releases every current waiter and arms the Signal for the next one.
func (s *Signal) Fire() {
	s.mu.Lock()
	defer s.mu.Unlock()
	close(s.ch)
	s.ch = make(chan struct{})
}

// Await blocks until pred holds, re-evaluating it on every Fire of any sig (a condition may advance from
// several sources), never on a clock. pred runs once up front; ctx is only a backstop that fails the
// test with what pred last saw.
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
		// Arm BEFORE evaluating: an event landing during pred() must not be lost.
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

// ansiRe matches CSI, OSC and two-byte escapes, the control plane stripped to make on-screen text
// matchable.
var ansiRe = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b\[[0-?]*[ -/]*[@-~]|\x1b[@-Z\\-_]`)

// squeeze keeps only the alphanumerics PTY output painted, in order, so a match survives hard wraps, box
// borders, padding and styling while still requiring the actual characters.
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

// Readable renders PTY output for a failure message (escapes stripped, blank rows collapsed): the reason a
// CLI died is usually on its last screen.
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

// PTYTap is a read-only client on a live PTY, implementing WSConn like the frontend, so a test can block
// on what the user would see on screen instead of sleeping.
type PTYTap struct {
	mu        sync.Mutex
	buf       []byte
	sig       *Signal
	closed    chan struct{}
	closeOnce sync.Once
}

// AttachPTY taps sessionID's PTY on its own goroutine (Attach blocks for the connection's life) and
// closes it on test cleanup. The tap never writes; drive input with the engine's Write.
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

// PTYAttacher is the one method of the terminal engine a tap needs. It is spelled
// with the engine's OWN WSConn so the engine's Engine interface satisfies it
// directly (Go does not unify interface-typed parameters structurally).
type PTYAttacher interface {
	Attach(ctx context.Context, sessionID string, conn terminal.WSConn) error
}

// A tap satisfies the engine's WSConn — the WebSocket shape it writes PTY output
// to — so the engine cannot tell a test from a browser.
var _ terminal.WSConn = (*PTYTap)(nil)

// WriteMessage appends a binary output frame's payload and fires the Signal; text (exit) frames carry no
// screen content.
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

// ReadMessage blocks until the tap is closed: a tap sends no input, and the
// engine's read pump must not spin. Returning an error is what lets Attach
// unwind cleanly on Close.
func (p *PTYTap) SetWriteDeadline(time.Time) error { return nil }

func (p *PTYTap) ReadMessage() (int, []byte, error) {
	<-p.closed
	return 0, nil, io.EOF
}

// Close detaches the tap, unblocking the engine's Attach. Idempotent: the engine
// also closes the conn from its write pump.
func (p *PTYTap) Close() error {
	p.closeOnce.Do(func() { close(p.closed) })
	return nil
}

// Screen returns everything the PTY has painted so far, raw (escapes included).
// Use it for failure messages; use WaitForScreen to match.
func (p *PTYTap) Screen() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return string(p.buf)
}

// Mark returns a cursor (bytes painted so far) for ScreenSince/WaitForScreenSince, so text typed before
// (a retried prompt) cannot match an old paint.
func (p *PTYTap) Mark() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.buf)
}

// ScreenSince returns everything painted after mark, raw.
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

// WaitForScreenSince blocks until the PTY paints needle AFTER mark.
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

// Signal exposes the tap's output edge so a caller can Await on a predicate that
// reads more than the screen (e.g. "the screen shows X **or** the runner row
// changed").
func (p *PTYTap) Signal() *Signal { return p.sig }

// Contains reports whether the PTY has painted needle, comparing on squeezed
// alphanumerics so wrapping/borders/styling cannot hide a match (see squeeze).
func (p *PTYTap) Contains(
	needle string,
) bool {
	return strings.Contains(squeeze(p.Screen()), squeeze(needle))
}

// ContainsAny reports whether the PTY has painted ANY of the needles, and which.
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

// WaitForScreen blocks until the PTY has painted needle, parking on real output
// frames between checks. ctx is a failure backstop only. It returns the raw
// screen for use in assertions.
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

// ScreenContains reports whether captured PTY output contains needle, comparing on
// squeezed alphanumerics so wrapping, box borders and styling cannot hide a match
// (see squeeze).
func ScreenContains(
	screen string,
	needle string,
) bool {
	return strings.Contains(squeeze(screen), squeeze(needle))
}

// EchoNeedle picks the TAIL of a typed prompt to look for: TUIs truncate or re-flow long input, and the
// tail proves the whole write landed.
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
