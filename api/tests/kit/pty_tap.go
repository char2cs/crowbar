//go:build integration

package kit

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
)

// Signal is a broadcast edge: every waiter parked on Wait() is released by the
// next Fire(). It is the primitive the integration harness uses to turn a real
// event (a PTY frame, a completed hook request) into something a test can BLOCK
// on — the alternative to sampling state on a clock.
//
// The Wait-then-check order is load-bearing and is why Fire swaps the channel
// rather than merely closing it: a caller takes the channel FIRST, then
// evaluates its predicate, then parks. An event landing in the gap between the
// evaluation and the park therefore closes a channel the caller is already
// holding, and the park returns immediately. Taking the channel after the check
// would lose exactly that wakeup.
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

// Await blocks until pred holds, re-evaluating it on every Fire of any of sigs,
// and returns pred's value. It NEVER samples on a clock: between evaluations it
// is parked on a real event. ctx is a failure BACKSTOP, not a synchronisation
// device — if it expires, Await fails the test with what pred last saw, rather
// than hanging until `go test -timeout` kills the binary with no diagnosis.
//
// pred is evaluated once up front, so a condition that is already true costs
// nothing and cannot deadlock waiting for an event that has already happened.
//
// Several sigs are allowed because a condition can legitimately be advanced by
// more than one kind of event — "the CLI bound a conversation (a hook landed) OR
// it is still asking us to trust the folder (it painted a dialog)" is one
// predicate over two independent sources, and parking on only one of them would
// miss the other.
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

// ansiRe matches the escape sequences a TUI paints its screen with (CSI, OSC,
// and the two-byte escapes). PTY output is a redraw stream, not a transcript:
// the same words are repainted inside box borders, at new cursor positions,
// wrapped at the terminal width. Stripping the control plane is the first half
// of making the TEXT on screen matchable.
var ansiRe = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b\[[0-?]*[ -/]*[@-~]|\x1b[@-Z\\-_]`)

// squeeze reduces PTY output to the letters and digits it painted, in order:
// escape sequences dropped, then every character that is not alphanumeric
// removed. It is what makes a substring match on a TUI screen RELIABLE rather
// than lucky.
//
// A terminal hard-wraps at the screen width, so any token can be split across
// two rows mid-character; Ink-style TUIs additionally repaint each row inside
// box-drawing borders and pad with spaces. A raw match for "FALCON-7719" can
// therefore fail against a screen that is plainly showing it (as "FALCON-77" /
// newline / "│ 19"). Squeezing both haystack and needle to their alphanumerics
// makes the match immune to wrapping, borders, padding and styling — the things
// that are noise here — while still requiring the actual characters, in order.
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

// Readable renders captured PTY output for a HUMAN reading a failure message:
// escape sequences stripped, blank rows collapsed. A raw dump is unreadable, and
// an unreadable failure message is why "the CLI did not start" gets misdiagnosed
// as "the CLI was slow" — the reason it actually died is usually printed right
// there on its last screen.
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

// PTYTap is a read-only client attached to a live PTY: it implements the
// terminal engine's WSConn exactly as the frontend's WebSocket does, so it sees
// precisely the bytes a user would see, and it turns them into a Signal a test
// can block on.
//
// This is the observable that replaces "sleep and hope the TUI caught up".
// Driving a real vendor CLI has two genuine synchronisation points — "the app
// has finished booting and is showing me its dialog" and "the app has taken the
// text I typed" — and both are visible ON SCREEN. Waiting for the screen to say
// so is a real signal; waiting 300ms is a guess about how fast the machine is.
type PTYTap struct {
	mu        sync.Mutex
	buf       []byte
	sig       *Signal
	closed    chan struct{}
	closeOnce sync.Once
}

// AttachPTY taps sessionID's PTY and returns the tap. The engine's Attach blocks
// for the life of the connection (it runs its read pump on the calling
// goroutine), so it is driven on its own goroutine here; the tap is closed on
// test cleanup, which is what unblocks it.
//
// The tap is a passive OBSERVER: it never writes to the PTY. Drive input with
// the terminal engine's Write (see Type), not through the tap.
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

// WriteMessage receives one PTY output frame ({"sessionId","data","snapshot"}),
// appends its payload to the tap's buffer and fires the tap's Signal. Every
// waiter re-checks its predicate against the new screen content.
func (p *PTYTap) WriteMessage(
	_ int,
	data []byte,
) error {
	var frame struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(data, &frame); err != nil {
		return nil //nolint:nilerr // a frame we cannot decode is not a transport failure; keep the tap attached.
	}
	if frame.Data == "" {
		return nil
	}
	p.mu.Lock()
	p.buf = append(p.buf, frame.Data...)
	p.mu.Unlock()
	p.sig.Fire()
	return nil
}

// ReadMessage blocks until the tap is closed: a tap sends no input, and the
// engine's read pump must not spin. Returning an error is what lets Attach
// unwind cleanly on Close.
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

// Mark returns a cursor into the output stream: the amount painted so far. Pass
// it to ScreenSince/WaitForScreenSince to match only what is painted AFTER this
// moment.
//
// This is what makes "wait until the app echoes what I just typed" honest. The
// screen is a scrollback of everything ever painted, so a bare Contains check
// for text the test has typed BEFORE (a retried prompt, a repeated codeword)
// would match the old paint and return before the app had taken the new input at
// all — the exact race the check exists to close.
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

// WaitForAnyScreen blocks until the PTY has painted any one of needles, and
// returns the one it saw. Use it when a real CLI has more than one legitimate
// way to reach the same state (e.g. it shows a trust dialog, or it was already
// trusted and went straight to its composer).
func (p *PTYTap) WaitForAnyScreen(
	t *testing.T,
	ctx context.Context,
	needles ...string,
) string {
	t.Helper()
	return Await(t, ctx, "PTY to paint any of "+strings.Join(needles, " | "), func() (string, bool) {
		return p.ContainsAny(needles...)
	}, p.sig)
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

// EchoNeedle picks the part of a typed prompt to look for in a TUI's composer. A
// TUI truncates a long prompt with an ellipsis or re-flows it, so the whole string
// is not guaranteed to appear verbatim; the TAIL is what the composer shows once it
// has taken ALL of the input, and it is the part that proves the whole write landed
// rather than a prefix of it.
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
