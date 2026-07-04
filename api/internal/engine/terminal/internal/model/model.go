// Package model defines the daemon-side terminal screen model and its serializer,
// quarantining the concrete headless VT emulator (charmbracelet/x/vt today, a
// different backend tomorrow) behind two interfaces so the session engine never
// depends on a specific emulator.
//
// The TerminalModel interface is the SESSION-facing surface: the session feeds raw
// PTY bytes in (Write/Resize/OnForegroundReset), asks for size/title/teardown, and
// reads the mid-sequence attach-boundary bytes via PendingInput. The Serializer
// reads a far wider surface (visible grid cells, cursor, alt-screen flag, scrollback,
// and the shadow private-mode/cursor/colour state) but does so ONLY against its
// paired concrete model inside this package, never through the session-facing
// interface. The two facts live on opposite sides of the package wall.
//
// Implementations are NOT goroutine-safe; the owning session serialises every access
// under its own lock (s.mu). The model deliberately adds no second lock.
//
// Backend reality note: the pinned x/vt commit exposes a subset of the callbacks the
// design envisioned. Private-mode, title, icon, alt-screen, cursor visibility, cursor
// style and default fg/bg/cursor-colour state ARE delivered via vt.Callbacks and mirrored
// in the shadow state. Scroll-region (DECSTBM), charset (SCS) and locking-shift (SI/SO) have
// NO callback at the pinned commit, so — per the spec §4.1 contract-note / step-10 fallback —
// the adapter parses those few sequences itself in Write (escan.go) and populates
// shadow.scrollTop/Bottom/scrollRegionSet and g0/g1/glLock. This matters in production: x/vt
// itself does NOT resolve an SO-invoked G1 line-drawing charset into its grid runes (verified
// at the pin). Step 10 re-emits the designation AND the active locking shift so that FUTURE
// live output after re-attach renders correctly — the scan is what makes that path live (it is
// not white-box-only dead code). It does NOT retroactively fix grid cells x/vt already painted
// via the unresolved G1 path: writeContent emits those raw cell bytes BEFORE the step-10
// designation, so any already-painted box-drawing self-heals on the app's next repaint rather
// than being exact on attach. Scrollback is sourced from x/vt's own built-in scrollback buffer
// rather than a pre-scroll eviction hook (which the pinned commit does not provide). The
// recorded spike findings and the adoption/deviation decisions live in model/UPSTREAM.md.
//
// Accepted residual — soft-wrap / reflow bit (spec §6.2 family, extending steps 5/7):
// the design declares soft-wrap preservation load-bearing — a row that soft-wrapped into
// the next physical row should be emitted at full cols width with NO trailing-blank trim
// and NO CR/LF separator, so the attached xterm keeps the wrapped/reflow attribute and
// identical physical-line semantics. The pinned x/vt commit exposes NO per-row wrap
// signal: ultraviolet's uv.Line is a flat []Cell with no isWrapped attribute, the visible
// grid offers no RowWrapped getter, and x/vt's built-in Scrollback even trims trailing
// cells off each pushed line, so there is no source from which the serializer could
// recover the wrap bit (the design's HARD P0 adoption gate for the wrap flag, §15, is
// therefore not met at this pin). The serializer consequently emits EVERY scrollback and
// grid row hard-broken with CR/LF and trimmed. The VISIBLE CELLS are preserved exactly —
// only the wrapped/reflow semantic is lost, so a width resize on the xterm.js client after
// attach reflows a previously-wrapped logical line differently from the model. The loss is
// bounded to that post-attach-resize corner and self-heals on the app's next full repaint
// (TUIs repaint on the SIGWINCH the attach implies; shells re-emit the prompt). Asserted as
// a tracked, non-regressing fact by TestSoftWrapResidualHardBreaksButPreservesCells. If a
// future x/vt commit surfaces the wrapped bit, steps 5/7 wrap-awareness is implemented then
// (and this residual is deleted).
package model

import "image/color"

// TerminalModel is a headless terminal emulator: an authoritative, in-memory mirror of
// the screen a correct terminal would currently display for a session. It is fed every
// PTY output byte and maintains the visible cell grid, bounded scrollback, cursor,
// active DEC private modes, alt-screen flag, and title. It performs no IO itself; it
// synthesizes device-query replies (CPR, DA, OSC color queries) but only ever hands
// them to the installed SetResponseSink, never writing them anywhere on its own
// (spec §3.8).
type TerminalModel interface {
	// Write feeds one chunk of raw PTY output into the emulator, advancing screen
	// state. Partial escape sequences split across chunk boundaries are buffered
	// internally and resumed on the next Write.
	Write(
		p []byte,
	)

	// Resize changes the grid dimensions to cols x rows. It reshapes the live grid
	// only; it does not reflow stored scrollback. The session calls Resize and
	// pty.Setsize under one lock hold with no intervening Write, preserving the
	// model==PTY size invariant.
	Resize(
		cols int,
		rows int,
	)

	// OnForegroundReset clears transient, app-owned screen state (alt-screen flag,
	// mouse tracking, focus reporting, bracketed paste, application cursor keys,
	// origin mode, cursor shape; autowrap back to ON) WITHOUT touching the grid,
	// scrollback, title, or cursor. The session calls it when the PTY foreground
	// process group returns to the shell so a later Serialize cannot leak stale modes
	// into an idle prompt. Idempotent.
	OnForegroundReset()

	// PendingInput returns the raw bytes of any incomplete escape/control sequence the
	// parser is currently mid-way through buffering, or an empty slice when the parser
	// is in ground state. Attach appends it after the clean redraw so a freshly
	// attached client re-syncs to the live mid-sequence boundary. It is a pure read.
	PendingInput() []byte

	// Title returns the current window title (last OSC 0/2 text), or "" if unset.
	Title() string

	// Cols returns the current grid width in columns.
	Cols() int

	// Rows returns the current grid height in rows.
	Rows() int

	// HeaderState returns, in one call, the four fields the persistence header needs:
	// grid cols and rows, the alt-screen flag, and the configured scrollback depth.
	HeaderState() (cols int, rows int, alt bool, scrollbackLines int)

	// ModelBytes returns the model's estimated resident size in bytes: the live cell
	// grid plus any retained scrollback cells. The session adds its own cached-blob
	// term on top (§9.4). It is a pure read the session calls under s.mu for the
	// engine's global memory-ceiling accounting.
	ModelBytes() int64

	// SetResponseSink installs the receiver for the emulator's device-query
	// answers (CPR, DA, OSC color queries). nil (the default) discards them —
	// correct while a live client xterm is the answerer (raw mode). The sink
	// is called from the model's internal drain goroutine; implementations of
	// the sink must be safe for that (the session's sink only calls
	// ptmx.Write, which is safe concurrently with reads).
	SetResponseSink(sink func(p []byte))

	// Close releases internal resources. Safe to call once; idempotent.
	Close()
}

// Serializer turns a TerminalModel's current state into ONE clean, well-formed,
// ground-state ANSI byte sequence that, when written to a fresh same-size terminal,
// reproduces the model's visible buffer exactly (cells, SGR, cursor, modes, title).
// The output contains no device-query requests, no device-query replies, no historical
// OSC re-emission beyond the single current title, and no dangling sequence.
type Serializer interface {
	// Serialize returns the redraw bytes for m. Implementations are deterministic for
	// a given model state. Serialize type-asserts m to its paired concrete model; a
	// mismatch is an unreachable programming error and panics (caught by the session's
	// recover wrapper, never by a bare unlock).
	Serialize(
		m TerminalModel,
	) []byte
}

// ThemeAware is an optional interface a TerminalModel may also implement to accept the
// host's terminal light/dark theme, so a foreground app (Claude Code's `auto` theme,
// vim, delta, …) can detect and follow it. It is kept off the core TerminalModel surface
// — and read via a guarded type assertion, like ModelHealth — so alternate backends and
// test fakes need not implement it.
//
// SetDefaultColors sets the colours an OSC 10/11 QUERY answers with (never the rendered
// grid — see the emulator SetDefault* contract). ThemeNotifyEnabled reports whether the
// foreground app subscribed to theme-change notifications via DEC private mode 2031, so the
// session knows whether emitting a CSI ?997;n report on a theme switch is wanted (and safe:
// a shell that never enabled 2031 must not receive one).
type ThemeAware interface {
	SetDefaultColors(bg, fg color.Color)
	ThemeNotifyEnabled() bool
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

// New constructs the matched (TerminalModel, Serializer) pair for the active backend.
// It is the single place the concrete backend is named. Returning both halves from one
// constructor is what makes the serializer's downcast of its paired model sound: the
// session stores the two returned interface values and can never combine a model of one
// backend with a serializer of another.
func New(
	cols int,
	rows int,
	scrollbackLines int,
) (TerminalModel, Serializer) {
	return newVTModel(cols, rows, scrollbackLines), vtSerializer{}
}
