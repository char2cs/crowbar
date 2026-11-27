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
	RowWrapped(
		y int,
	) bool
	ScrollbackRowWrapped(
		i int,
	) bool
	Close()
	SetResponseSink(f func(p []byte))
	SetDefaultBackgroundColor(c color.Color)
	SetDefaultForegroundColor(c color.Color)
}

type vtEmu struct {
	*vt.Emulator
	drainDone chan struct{}
	sink      atomic.Pointer[func(p []byte)]
}

func (v *vtEmu) Write(
	p []byte,
) {
	if _, err := v.Emulator.Write(p); err != nil {
		slog.Warn("terminal: emulator refused output", "err", err)
	}
}

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

func (v *vtEmu) SetResponseSink(f func(p []byte)) {
	if f == nil {
		v.sink.Store(nil)
		return
	}
	v.sink.Store(&f)
}

func (v *vtEmu) Close() {
	if pw, ok := v.Emulator.InputPipe().(io.Closer); ok { //nolint:staticcheck // QF1008: explicit Emulator qualifier mirrors the mandatory v.Emulator.Write above and documents that InputPipe is promoted from the embedded emulator.
		if err := pw.Close(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
			slog.Warn("terminal: close emulator input pipe", "err", err)
		}
	}
	<-v.drainDone
}

func (v *vtEmu) ScrollbackLine(
	y int,
) uv.Line {
	return v.Emulator.Scrollback().Line(y)
}

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
