package model

import (
	"errors"
	"image/color"
	"io"
	"log/slog"
	"sync/atomic"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
)

// emulator is the file-local seam over the concrete VT emulator, so a unit test can reach the
// parse-panic recovery with a panicking fake. It adds no lock; the session's lock is the only one.
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
	// RowWrapped / ScrollbackRowWrapped report xterm's isWrapped for a grid row or a
	// scrollback line (0 = oldest): it continues the row above by autowrap.
	RowWrapped(
		y int,
	) bool
	ScrollbackRowWrapped(
		i int,
	) bool
	// Close releases the emulator (and, for the production emulator, stops its
	// response-pipe drain goroutine). Idempotent.
	Close()
	// SetResponseSink installs the receiver for device-query replies drained from
	// the emulator's response pipe. nil discards them. See vtEmu.SetResponseSink.
	SetResponseSink(f func(p []byte))
	// SetDefault*Color set only what OSC 11 / OSC 10 queries answer with: no cell colour, no callback, so the
	// serialized grid never changes.
	SetDefaultBackgroundColor(c color.Color)
	SetDefaultForegroundColor(c color.Color)
}

type vtEmu struct {
	*vt.Emulator
	drainDone chan struct{}
	// sink receives each reply drained off the InputPipe, or nil to discard; an atomic.Pointer because the
	// drain goroutine reads it.
	sink atomic.Pointer[func(p []byte)]
}

// Write feeds bytes to the emulator; a refusal (only once closed) is logged. x/vt answers device queries
// into an UNBUFFERED pipe, so without the drainResponses reader that reply would block this Write under
// the session lock; Write returns only once any reply has been drained.
func (v *vtEmu) Write(
	p []byte,
) {
	if _, err := v.Emulator.Write(p); err != nil {
		slog.Warn("terminal: emulator refused output", "err", err)
	}
}

// drainResponses hands each reply x/vt writes to its InputPipe to the sink, or discards it when none is
// installed. It exits on io.EOF after Close and signals drainDone.
func (v *vtEmu) drainResponses() {
	defer close(v.drainDone)
	buf := make([]byte, 512)
	for {
		n, err := v.Emulator.Read(buf) //nolint:staticcheck // QF1008: explicit Emulator qualifier mirrors the mandatory v.Emulator.Write above and documents that Read is promoted from the embedded emulator.
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

// SetResponseSink installs f as the device-query reply receiver; nil reverts to discarding. Safe against
// the drain goroutine (atomic swap).
func (v *vtEmu) SetResponseSink(f func(p []byte)) {
	if f == nil {
		v.sink.Store(nil)
		return
	}
	v.sink.Store(&f)
}

// Close ends drainResponses and joins it. It does NOT call Emulator.Close, whose unguarded `closed` write
// races the drain's Read inside x/vt; closing the pipe's writer gives the drain io.EOF instead.
func (v *vtEmu) Close() {
	if pw, ok := v.Emulator.InputPipe().(io.Closer); ok { //nolint:staticcheck // QF1008: explicit Emulator qualifier mirrors the mandatory v.Emulator.Write above and documents that InputPipe is promoted from the embedded emulator.
		if err := pw.Close(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
			slog.Warn("terminal: close emulator input pipe", "err", err)
		}
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
