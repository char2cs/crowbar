// Package model defines the daemon-side terminal screen model and serializer, hiding the concrete VT
// emulator behind interfaces. Not goroutine-safe: the session serialises access under its own lock.
// Sequences the pinned backend gives no callback for are scanned in Write (escan.go); see UPSTREAM.md.
package model

import "image/color"

// TerminalModel is a headless emulator mirroring the screen a correct terminal would show. It does no
// IO: device-query replies go only to the installed SetResponseSink.
type TerminalModel interface {
	// Write feeds one chunk of raw PTY output into the emulator, advancing screen
	// state. Partial escape sequences split across chunk boundaries are buffered
	// internally and resumed on the next Write.
	Write(
		p []byte,
	)

	// Resize reflows to cols x rows as the client's xterm.js does. The session pairs it with
	// pty.Setsize under one lock with no intervening Write, keeping model and PTY the same size.
	Resize(
		cols int,
		rows int,
	)

	// OnForegroundReset clears app-owned transient modes (alt-screen, mouse, paste, cursor keys…)
	// without touching grid, scrollback, title or cursor, so a later Serialize cannot leak stale modes
	// into an idle shell prompt. Idempotent.
	OnForegroundReset()

	// Title returns the current window title (last OSC 0/2 text), or "" if unset.
	Title() string

	// Cols returns the current grid width in columns.
	Cols() int

	// Rows returns the current grid height in rows.
	Rows() int

	// HeaderState returns, in one call, the four fields the persistence header needs:
	// grid cols and rows, the alt-screen flag, and the configured scrollback depth.
	HeaderState() (cols, rows int, alt bool, scrollbackLines int)

	// ModelBytes estimates resident size (grid plus scrollback cells) for the engine's global memory
	// ceiling; a pure read under the session lock.
	ModelBytes() int64

	// SetResponseSink installs the receiver for device-query answers; nil discards them, correct while a
	// live client xterm answers. The sink runs on the model's drain goroutine and must be safe there.
	SetResponseSink(sink func(p []byte))

	// Close releases internal resources. Safe to call once; idempotent.
	Close()
}

// Serializer turns a model's state into one ground-state ANSI sequence that reproduces the visible
// buffer on a fresh same-size terminal, with no device queries, replies or dangling sequences.
type Serializer interface {
	// Serialize is deterministic per model state. It panics on a model that is not its paired concrete
	// type, an unreachable programming error caught by the session's recover wrapper.
	Serialize(
		m TerminalModel,
	) []byte
}

// ThemeAware optionally accepts the host's light/dark theme so foreground apps can follow it; kept off
// TerminalModel so fakes need not implement it. ThemeNotifyEnabled reports DEC mode 2031, because a
// shell that never enabled it must not receive a theme-change report.
type ThemeAware interface {
	SetDefaultColors(bg, fg color.Color)
	ThemeNotifyEnabled() bool
}

// ScreenReader optionally renders the VISIBLE grid as plain text, for inspecting a CLI blocked on a
// modal no hook reports. Viewport only: a prompt scrolled out of sight is not blocking anything.
type ScreenReader interface {
	// ScreenText renders the visible grid as plain text: one line per row, trailing
	// blanks trimmed, rows joined by "\n". Content only — no SGR, no cursor, no
	// scrollback.
	ScreenText() string
}

// ModelHealth is an optional interface a TerminalModel may also implement to expose its
// parse-health to observability without widening the session-facing surface. The
// session reads it with a guarded type assertion.
type ModelHealth interface {
	// Degraded reports whether the model recovered at least one parse panic this
	// session (the screen was blanked and re-parsed).
	Degraded() bool

	// ParsePanics returns the count of recovered emulator-write panics this session.
	ParsePanics() int
}

// New constructs the matched (TerminalModel, Serializer) pair, the one place the backend is named;
// one constructor is what makes the serializer's downcast of its paired model sound.
func New(
	cols int,
	rows int,
	scrollbackLines int,
) (TerminalModel, Serializer) {
	return newVTModel(cols, rows, scrollbackLines), vtSerializer{}
}
