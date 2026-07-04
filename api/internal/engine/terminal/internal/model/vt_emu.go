package model

import (
	"image/color"
	"io"
	"sync/atomic"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
)

// emulator is the narrow, file-local seam over the concrete headless VT emulator
// (*vt.Emulator today). vtModel holds this interface rather than the concrete type for
// two reasons: it makes the parse-panic recover branch reachable from a unit test that
// substitutes an emulator whose Write panics on a sentinel byte, and it keeps the
// concrete emulator type quarantined. The seam adds no lock; the session's lock remains
// the sole synchronisation.
type emulator interface {
	Write(
		p []byte,
	)
	Resize(
		cols int,
		rows int,
	)
	Width() int
	Height() int
	CellAt(
		x int,
		y int,
	) *uv.Cell
	CursorPosition() uv.Position
	IsAltScreen() bool
	ScrollbackLen() int
	ScrollbackLine(
		y int,
	) uv.Line
	// Close releases the emulator (and, for the production emulator, stops its
	// response-pipe drain goroutine). Idempotent.
	Close()
	// SetResponseSink installs the receiver for device-query replies drained from
	// the emulator's response pipe. nil discards them. See vtEmu.SetResponseSink.
	SetResponseSink(f func(p []byte))
	// SetDefaultBackgroundColor / SetDefaultForegroundColor set the terminal's
	// default colours — the values an OSC 11 / OSC 10 QUERY answers with. They
	// deliberately do NOT touch the per-cell rendering colour (e.bgColor/fgColor)
	// and fire no callback, so setting them changes only what a querying app reads,
	// never the serialized grid. Promoted from the embedded *vt.Emulator.
	SetDefaultBackgroundColor(c color.Color)
	SetDefaultForegroundColor(c color.Color)
}

type vtEmu struct {
	*vt.Emulator
	drainDone chan struct{}
	// sink receives each reply chunk drainResponses reads off the InputPipe, or is
	// nil to discard (today's raw-mode behaviour: the live client xterm answers
	// device queries instead). Set via SetResponseSink; read from the drain
	// goroutine, so it is an atomic.Pointer rather than a plain field.
	sink atomic.Pointer[func(p []byte)]
}

// Write feeds bytes to the underlying emulator, discarding its (n, err) return. The
// emulator performs no IO, so a write to its internal grid never fails meaningfully.
//
// The pinned x/vt is NOT fully headless: its handlers answer device queries — Device
// Attributes (ESC[c / ESC[>c), Device-Status / Cursor-Position reports (ESC[6n), and
// colour-OSC queries — by writing the reply into an UNBUFFERED io.Pipe (InputPipe). A real
// app emits those queries to its stdout constantly (vim, tmux, etc.), so without a reader
// that reply write would block INSIDE this Write — which the session runs under its lock —
// deadlocking every later Attach/Serialize. The drainResponses goroutine (started in
// newEmulator) is that reader: it discards the replies, because this passive shadow must
// never answer queries (the live front-end xterm client answers them and feeds the reply
// back over the PTY). The unbuffered pipe also gives a useful guarantee for tests: this
// Write only RETURNS once the drain goroutine has consumed any reply.
func (v *vtEmu) Write(
	p []byte,
) {
	_, _ = v.Emulator.Write(p)
}

// drainResponses consumes the bytes x/vt auto-writes to its InputPipe in reply to device
// queries (see Write) and hands each complete read to the installed sink, or discards it
// when no sink is installed (raw mode: the live client xterm is the answerer, spec §3.8).
// It exits when Close shuts the pipe (Read → io.EOF), signalling completion on drainDone
// so Close can join it.
func (v *vtEmu) drainResponses() {
	defer close(v.drainDone)
	buf := make([]byte, 512)
	for {
		n, err := v.Emulator.Read(buf)
		if n > 0 {
			if sink := v.sink.Load(); sink != nil {
				(*sink)(append([]byte(nil), buf[:n]...))
			}
		}
		if err != nil {
			return
		}
	}
}

// SetResponseSink installs f as the receiver of device-query replies drained from the
// emulator's response pipe (see drainResponses). Passing nil reverts to discarding them.
// Safe to call concurrently with the drain goroutine (atomic.Pointer swap); the session
// calls it once at spawn, under its own lock, well before any Write can race it.
func (v *vtEmu) SetResponseSink(f func(p []byte)) {
	if f == nil {
		v.sink.Store(nil)
		return
	}
	v.sink.Store(&f)
}

// Close ends drainResponses and waits for it, so a closed emulator leaves no lingering
// reader. It deliberately does NOT call Emulator.Close: at the pinned commit that method
// writes the emulator's unguarded `closed` bool, which races the drain goroutine's
// concurrent Read of the same bool (an x/vt-internal data race we cannot fix). Instead it
// closes the InputPipe's writer end, which makes the drain's Read return io.EOF — unblocking
// and ending the goroutine — WITHOUT touching `closed`. The emulator is being discarded
// (model.Close / recreateEmu), and Close is serialised with Write by the session lock, so no
// further Write races this teardown.
func (v *vtEmu) Close() {
	if pw, ok := v.Emulator.InputPipe().(io.Closer); ok {
		_ = pw.Close()
	}
	<-v.drainDone
}

// ScrollbackLine returns a copy of scrollback line y (0 = oldest), or nil when out of
// bounds. x/vt's built-in scrollback is the authoritative source at the pinned commit.
func (v *vtEmu) ScrollbackLine(
	y int,
) uv.Line {
	return v.Emulator.Scrollback().Line(y)
}

// newEmulator constructs the production emulator. It is a package-level var only so a
// unit test can substitute a panicking emulator to drive the parse-panic recover branch;
// production never reassigns it. buildEmu is its sole caller.
var newEmulator = func(
	cols int,
	rows int,
	cb vt.Callbacks,
	scrollbackLines int,
) emulator {
	e := vt.NewEmulator(cols, rows)
	e.SetCallbacks(cb)
	if scrollbackLines > 0 {
		e.SetScrollbackSize(scrollbackLines)
	}
	v := &vtEmu{Emulator: e, drainDone: make(chan struct{})}
	go v.drainResponses()
	return v
}
