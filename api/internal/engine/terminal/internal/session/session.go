package session

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/engine/terminal/internal/model"
)

const clientSendBuf = 256

// newModel is the model-construction seam. It is a package-level var only so a unit test can
// substitute a model whose Resize/Serialize panics, driving the §8.5 session recover
// backstops (mutateModelLocked/serializeLocked) through Session.Resize/Attach — the real
// vtModel recovers Write panics internally, so those session backstops are otherwise
// unreachable from a test. Production never reassigns it; spawn is its sole caller.
var newModel = model.New

// defaultScrollbackLines is the scrollback depth a create/restore with no explicit
// value resolves to — mirroring the frontend's terminalScrollback default (§9.1).
const defaultScrollbackLines = 10000

// foregroundSampleInterval debounces the per-chunk foreground-process-group sample so
// the TIOCGPGRP ioctl (and the app-death-edge model teardown) stays off the hot path
// while still converging within ~¼ s of an app exiting (§11.1 sampling site #1).
const foregroundSampleInterval = 250 * time.Millisecond

// minEmitInterval is the model-driven frame clock (spec §3.3): interactive
// deltas emit immediately; bursts coalesce to at most one frame per interval.
const minEmitInterval = 8 * time.Millisecond

// responseReplyBufDepth bounds the device-query reply queue that decouples the
// model's response sink from the blocking ptmx.Write (spec §3.8, C1). A hostile
// or broken foreground app can emit queries faster than it drains its own stdin;
// once the queue and the PTY input buffer are both full, further replies are
// dropped (a lost query answer times out, it never wedges the session lock).
const responseReplyBufDepth = 64

// OutputFrame is a chunk of PTY output delivered to attached clients.
//
// Snapshot marks a self-contained ground-state redraw (the serialized model)
// rather than incremental PTY bytes: the client must RESET its local buffer
// before applying Data, replacing whatever it accumulated — the mechanism
// behind both the attach redraw and the post-resize resync.
type OutputFrame struct {
	SessionID string
	Data      []byte
	Snapshot  bool
}

// client represents one attached WebSocket subscriber.
type client struct {
	send chan OutputFrame
}

// Session is a single PTY session. It may be live (ptmx != nil, model != nil) or
// suspended (ptmx == nil, model == nil, created via NewPlaceholder which holds only the
// persisted rawBlob).
type Session struct {
	id         string
	ptmx       *os.File
	cmd        *exec.Cmd
	model      model.TerminalModel
	serializer model.Serializer
	mu         sync.Mutex
	flushMu    sync.Mutex
	clients    map[*client]struct{}
	done       chan struct{}
	once       sync.Once
	suspending bool
	dirty      bool
	exitCode   int
	cwd        string
	shell      string
	profileID  string
	// lastBlob caches the last live-session serialized blob (header + redraw) so a
	// cadence flush of an unchanged session reuses it and skips the grid render (§8.4).
	// It is reclaimable under memory pressure (DropCachedBlob, §9.4).
	lastBlob []byte
	// rawBlob is the placeholder's persisted serialized blob, returned verbatim by the
	// model-less Snapshot fast-path. Distinct from lastBlob and never aliased (§8.4).
	rawBlob []byte
	// modelPanics counts recovered SESSION-LEVEL model-access panics — the §8.5
	// backstops around Resize/Serialize/Emit/Prime/teardown — surfaced via Stats
	// and never fatal. It does NOT count vtModel.Write's internal parse panics,
	// which the model recovers itself (recreateEmu) while staying model-driven.
	modelPanics int
	// lastForegroundPgid latches the previous foreground process-group sample so the
	// app→shell return edge fires OnForegroundReset exactly once (§11.1).
	lastForegroundPgid int
	lastFgSampleAt     time.Time
	// sampleForegroundLocked is the §11.1 site #1 foreground-process-group probe that
	// pumpStep runs strictly LAST in its critical section (after fan-out and the model
	// write). It is a field — wired to checkForegroundResetLocked in newBareSession —
	// solely so a test can substitute a deterministic hooked sampler to pin the
	// fan-out → model-write → foreground-sample ordering the spec mandates (§11.1 site #1).
	sampleForegroundLocked func()
	// Model-driven output (spec 2026-07-03): every live session is model-driven —
	// clients receive model-derived diff/keyframe frames, never raw PTY bytes.
	// Raw streaming survives ONLY as the degraded fallback. modelPanics counts
	// solely the SESSION-LEVEL model-access panics the §8.5 backstops recover
	// (resize/serialize/emit/prime); a nonzero count (or a nil model on a
	// placeholder before restore) flips the session to raw streaming for its
	// remaining lifetime. It deliberately does NOT count the model's internal
	// parse panics: vtModel.Write recovers those itself (fresh emulator, blanked
	// screen) and stays model-driven — a self-heal, not a fallback. emitter state
	// is guarded by s.mu like the model.
	emitter *model.DiffEmitter
	// modelDrivenFellBack latches the raw-fallback log so a degraded session logs the
	// flip exactly once instead of once per chunk.
	modelDrivenFellBack bool
	// emitForTest, when non-nil, replaces s.emitter.Emit inside emitLocked. It exists
	// solely so a test can make the EMIT path panic (writeModelLocked having already
	// succeeded) while staying inside emitLocked's own recover scope — a state no
	// adversarial PTY input can reach deterministically, since the emitter's Emit never
	// panics on real model state. Production never sets it.
	emitForTest func(m model.TerminalModel) ([]byte, bool)
	// Adaptive frame clock (spec §3.3): emits immediately when the last emit
	// is older than minEmitInterval (interactive echo stays un-batched), else
	// arms one trailing timer at the boundary so bursts coalesce. Guarded by
	// s.mu; the timer callback re-locks.
	lastEmitAt time.Time
	emitTimer  *time.Timer

	// Theme-notify dedupe (SetTheme): themeEmitted latches once a CSI ?997;n report has
	// been emitted, themeEmittedDark records the last-emitted polarity. Together they
	// gate the report to the first subscribed push and every subsequent light<->dark
	// FLIP, so an unrelated same-polarity theme tweak never spams a running app with
	// redundant re-query triggers. Guarded by s.mu.
	themeEmitted     bool
	themeEmittedDark bool
}

// newBareSession allocates a Session shell with no PTY and no model. New/NewRestored fill
// it in via spawn; NewPlaceholder stores only its rawBlob.
func newBareSession(
	id string,
	shell string,
	cwd string,
	profileID string,
) *Session {
	s := &Session{
		id:        id,
		clients:   make(map[*client]struct{}),
		done:      make(chan struct{}),
		cwd:       cwd,
		shell:     shell,
		profileID: profileID,
		exitCode:  -1,
	}
	s.sampleForegroundLocked = s.checkForegroundResetLocked
	return s
}

// spawnParams carries exactly one of the two birth modes (§9.1): create (Blob == nil,
// size+scrollback from Cols/Rows/ScrollbackLines) or restore (Blob != nil, size+scrollback
// parsed from the blob's CRWB1 header, the rest ignored).
type spawnParams struct {
	Cols            int
	Rows            int
	ScrollbackLines int
	Blob            []byte
}

// New spawns a PTY subprocess at cols×rows, builds the screen model at that size and the
// resolved scrollback depth, and starts the io pump. Every session spawned this way is
// model-driven (spec 2026-07-03): clients receive model-derived frames, and raw streaming
// survives only as the degraded fallback.
func New(
	id string,
	shell string,
	cwd string,
	profileID string,
	env []string,
	cols int,
	rows int,
	scrollbackLines int,
) (*Session, error) {
	s := newBareSession(id, shell, cwd, profileID)
	if err := s.spawn(env, spawnParams{Cols: cols, Rows: rows, ScrollbackLines: scrollbackLines}); err != nil {
		return nil, err
	}
	return s, nil
}

// NewRestored spawns a PTY subprocess and rebuilds the screen model from a persisted blob:
// spawn parses the blob's CRWB1 header for the authoritative size+scrollback (§12), sizes
// the PTY to it BEFORE the first read, and feeds the redraw bytes into the fresh model
// before the pump starts so the restored screen is reproduced exactly.
func NewRestored(
	id string,
	shell string,
	cwd string,
	profileID string,
	env []string,
	rawBlob []byte,
) (*Session, error) {
	s := newBareSession(id, shell, cwd, profileID)
	if err := s.spawn(env, spawnParams{Blob: rawBlob}); err != nil {
		return nil, err
	}
	return s, nil
}

// NewPlaceholder builds a suspended Session with no live PTY and no model. The serialized
// rawBlob is retained so a later restore re-reads it; Snapshot returns it verbatim. No pump
// goroutine is started — a later Attach→restore spawns a fresh NewRestored session.
func NewPlaceholder(
	id string,
	shell string,
	cwd string,
	profileID string,
	rawBlob []byte,
) *Session {
	s := newBareSession(id, shell, cwd, profileID)
	s.rawBlob = rawBlob
	return s
}

// spawn is the single PTY-birth helper shared by the create and restore paths. It starts
// the PTY, sizes it to the resolved dimensions BEFORE the first read (preserving the
// model==PTY size invariant, §4.2), builds the model+serializer at that size, replays the
// restore redraw (if any) into the model, and launches the pump goroutine.
func (s *Session) spawn(
	env []string,
	p spawnParams,
) error {
	cols, rows, sbLines, redraw := s.resolveBirth(p)

	cmd := exec.Command(s.shell) //nolint:gosec // G204: s.shell is the operator-configured login shell path, not attacker-controlled; spawning it is the whole point of a terminal session.
	cmd.Dir = s.cwd
	cmd.Env = env

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("session: pty start: %w", err)
	}

	// Size the PTY to the persisted/requested dimensions before any Read so the shell's
	// first output is generated at the correct width.
	_ = pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)}) //nolint:gosec // G115: cols/rows are terminal dimensions (resolveCols/Rows floor them at 1); pty.Winsize fields are uint16 by definition.

	m, ser := newModel(cols, rows, sbLines)
	if len(redraw) > 0 {
		m.Write(redraw)
	}

	s.ptmx = ptmx
	s.cmd = cmd
	s.model = m
	s.serializer = ser
	s.emitter = model.NewDiffEmitter()
	if s.model != nil {
		s.startResponseSink(s.ptmx)
	}

	go s.pump()
	return nil
}

// startResponseSink wires the model's device-query response path (spec §3.8) so
// the blocking ptmx.Write NEVER runs on the model's drain goroutine while s.mu is
// held. That decoupling is load-bearing (C1): x/vt answers CPR/DA/colour queries
// by writing into an UNBUFFERED reply pipe drained by vtEmu.drainResponses, which
// then invokes this sink SYNCHRONOUSLY. If the sink did ptmx.Write directly, a
// hostile app that emits queries (e.g. a loop printing ESC[6n) without reading its
// own stdin would fill the PTY input queue, block ptmx.Write inside the drain,
// stall the drain's next pipe read, block emu.Write inside pumpStep's model write
// (which holds s.mu), and permanently wedge Kill/Snapshot/Shutdown — engine-wide.
//
// Instead: a bounded queue + one writer goroutine own the blocking write OFF the
// lock, and the sink only does a NON-BLOCKING send. When both the queue and the
// PTY input buffer saturate, replies are dropped rather than blocking — a dropped
// answer merely times that one query out. The writer exits when s.done closes
// (Kill closes ptmx first, unblocking any in-flight ptmx.Write with an error, then
// shutdown closes s.done). replyCh is never closed, so the sink's post-teardown
// non-blocking sends harmlessly fill/drop instead of panicking on a closed channel.
func (s *Session) startResponseSink(
	ptmx *os.File,
) {
	replyCh := make(chan []byte, responseReplyBufDepth)
	done := s.done
	safego.Go("terminal.session.replyWriter", func() {
		for {
			select {
			case reply := <-replyCh:
				// Ignore write errors: a failing write means the PTY is going
				// away, so keep draining replyCh (the sink must never block)
				// until s.done releases the goroutine.
				_, _ = ptmx.Write(reply)
			case <-done:
				return
			}
		}
	})
	s.model.SetResponseSink(func(reply []byte) {
		// reply is a fresh per-call allocation from vtEmu.drainResponses
		// (append([]byte(nil), buf[:n]...)), so we own it outright — no copy
		// needed before handing it to the writer goroutine.
		select {
		case replyCh <- reply:
		default:
			// Queue full and the PTY input buffer is backed up too; drop the
			// reply rather than block the drain goroutine (see the doc above).
		}
	})
}

// resolveBirth returns the size, scrollback depth, and restore redraw bytes for a spawn,
// applying the §9.1 defaults (80×24, scrollback 10000) and, for the restore path, parsing
// the CRWB1 header.
func (s *Session) resolveBirth(
	p spawnParams,
) (cols, rows, sbLines int, redraw []byte) {
	if p.Blob != nil {
		hc, hr, hsb, body := parseBlob(p.Blob)
		return resolveCols(hc), resolveRows(hr), resolveScrollback(hsb), body
	}
	return resolveCols(p.Cols), resolveRows(p.Rows), resolveScrollback(p.ScrollbackLines), nil
}

func resolveCols(
	c int,
) int {
	if c <= 0 {
		return 80
	}
	return c
}

func resolveRows(
	r int,
) int {
	if r <= 0 {
		return 24
	}
	return r
}

func resolveScrollback(
	n int,
) int {
	if n <= 0 {
		return defaultScrollbackLines
	}
	return n
}

// parseBlob splits a persisted blob into its CRWB1 header fields and the redraw body. A
// malformed/absent header (including a stale raw .buf) returns zero size and a nil body,
// which resolveBirth treats as an empty session at the default size (§12, no migration).
func parseBlob(
	blob []byte,
) (cols, rows, scrollbackLines int, body []byte) {
	nl := bytes.IndexByte(blob, '\n')
	if nl < 0 {
		return 0, 0, 0, nil
	}
	header := string(blob[:nl])
	var alt int
	n, err := fmt.Sscanf(header, "CRWB1 %d %d %d %d", &cols, &rows, &alt, &scrollbackLines)
	if err != nil || n != 4 {
		return 0, 0, 0, nil
	}
	return cols, rows, scrollbackLines, blob[nl+1:]
}

// ID returns the session identifier.
func (s *Session) ID() string {
	return s.id
}

// Done returns a channel closed when the session has terminated.
func (s *Session) Done() <-chan struct{} {
	return s.done
}

// IsLive reports whether the PTY is still open (ptmx != nil).
func (s *Session) IsLive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ptmx != nil
}

// AttachedCount returns the number of currently attached clients.
func (s *Session) AttachedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clients)
}

// State returns one of "active", "detached", or "suspended".
func (s *Session) State() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptmx == nil {
		return "suspended"
	}
	if len(s.clients) > 0 {
		return "active"
	}
	return "detached"
}

// CWD returns the last known working directory (updated from OSC 7 sequences).
func (s *Session) CWD() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cwd
}

// Shell returns the shell binary path.
func (s *Session) Shell() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shell
}

// ProfileID returns the terminal profile identifier.
func (s *Session) ProfileID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.profileID
}

// FlushMu returns the dedicated flush mutex used by the engine to serialise bulk
// persistence with the cadence flush. It is separate from s.mu; Snapshot takes s.mu
// INSIDE the flushMu hold, never the reverse (§8.4).
func (s *Session) FlushMu() *sync.Mutex {
	return &s.flushMu
}

// IsIdle reports whether the shell is idle (no foreground child process).
func (s *Session) IsIdle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isIdleLocked()
}

// Attach registers a new client and returns its channel, pre-filled with ONE clean
// ground-state redraw serialized from the current model (§8.3/Appendix A). No raw replay,
// no DEC-mode preamble: the serialized state is self-contained, query-free, and fully
// terminated. The buffered mid-sequence partial is appended after the redraw so the new
// client's fresh parser converges to the same boundary the live clients hold.
func (s *Session) Attach() (<-chan OutputFrame, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-s.done:
		return nil, fmt.Errorf("session: attach: session %s is dead", s.id)
	default:
	}

	cl := &client{send: make(chan OutputFrame, clientSendBuf)}

	if s.model != nil { //nolint:nestif // ordered attach protocol (flush-then-snapshot vs raw-continuation) documented inline; flattening would break the required sequence.
		// Sample the foreground-reset detector before serializing (§11.1 site #2) so a
		// re-attach inside the pumpStep debounce window of a SIGKILLed app never bakes its
		// stale alt/mouse modes into the new client.
		s.checkForegroundResetLocked()
		if s.modelEmitHealthyLocked() {
			// Model-driven Attach re-bases the emitter to the CURRENT model
			// state, which is safe for the new client but would silently drop
			// any delta accumulated since the last Emit/Prime for EXISTING
			// clients (the attach snapshot is not fanned out to them). Flush
			// that pending delta to existing clients FIRST — through the same
			// emitFrameLocked path the pump uses — so no output is lost, THEN
			// serialize the fresh snapshot for the new client and Prime.
			//
			// flushPendingEmitLocked (Task 7) also disarms any trailing frame-
			// clock timer here: without that, a burst chunk could still be
			// sitting behind an unfired 8ms timer, and letting it fire AFTER
			// this attach's own snapshot/Prime would emit a stale diff off the
			// wrong base straight into the new client's freshly-primed state.
			s.flushPendingEmitLocked()
		}
		redraw := s.serializeLocked()
		if !s.modelEmitHealthyLocked() {
			// Append the buffered mid-sequence partial ONLY on the degraded/raw
			// path. There the client keeps receiving live PTY bytes, so the
			// CONTINUATION of that partial genuinely arrives after this snapshot —
			// priming the fresh client's parser with the partial makes it converge
			// to the same boundary the live clients hold.
			//
			// Under healthy model-driven emission the client receives only model-
			// DERIVED frames; the raw continuation bytes NEVER come. Appending the
			// partial would strand the client's parser mid-escape forever — a
			// truncated OSC title committed as the window title, or a mid-rune byte
			// surfacing as U+FFFD once json.Marshal re-encodes it. The serializer's
			// output is already self-contained, query-free and fully terminated, so
			// a healthy attach snapshot ends exactly there.
			redraw = append(redraw, s.model.PendingInput()...)
		}
		if len(redraw) > 0 {
			cl.send <- OutputFrame{SessionID: s.id, Data: redraw, Snapshot: true}
		}
		if s.modelEmitHealthyLocked() {
			s.primeLocked()
		}
	}

	s.clients[cl] = struct{}{}
	return cl.send, nil
}

// Resync re-emits the serialized ground-state redraw to every attached client
// as a Snapshot frame. It is the post-resize convergence path: xterm's
// client-side reflow deposits stale copies of a repainting TUI into the LOCAL
// scrollback on every resize, while this model never reflows — so replacing
// the client buffer with the model state removes the junk.
//
// Gated on a foreground app being present: at an idle shell prompt xterm's
// native reflow is already correct (append-only output) and a resync would
// only cost the client its scroll position. Returns true when a resync was
// REQUESTED/attempted (a foreground app was present and not idle), even if the
// model-driven emit path then panicked into raw fallback — not a guarantee that
// bytes reached every client. Overflow handling differs by branch: the healthy
// model-driven branch fans the keyframe out via fanOutFrameLocked, which
// DISCONNECTS a client whose send buffer is full (drop-on-overflow; it re-attaches
// to a fresh keyframe). Only the degraded RAW branch below SKIPS a full client
// rather than blocking or disconnecting — it is already saturated with raw output
// that supersedes this snapshot.
func (s *Session) Resync() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.model == nil || s.isIdleLocked() {
		return false
	}
	s.checkForegroundResetLocked()

	if s.modelEmitHealthyLocked() {
		// One mechanism: invalidate the diff base so emitFrameLocked's next
		// Emit demands a keyframe, then let it serialize+fan out+re-prime —
		// the exact same path the pump uses for a post-resize keyframe.
		//
		// Cancel any armed trailing frame-clock timer (Task 7) FIRST: the
		// keyframe below is serialized from the current model, which already
		// reflects every chunk written so far (the clock only defers the
		// EMIT, never the model write) — so it subsumes whatever delta the
		// timer was going to flush. Leaving the timer armed would just let it
		// fire later and emit a redundant/stale diff off the base this
		// keyframe just re-primed.
		s.stopEmitTimerLocked()
		s.emitter.Invalidate()
		s.emitFrameLocked()
		s.lastEmitAt = time.Now()
		return true
	}

	redraw := s.serializeLocked()
	redraw = append(redraw, s.model.PendingInput()...)
	if len(redraw) == 0 {
		return false
	}
	for cl := range s.clients {
		select {
		case cl.send <- OutputFrame{SessionID: s.id, Data: redraw, Snapshot: true}:
		default:
		}
	}
	return true
}

// Detach removes a client from the fan-out set and closes its channel.
func (s *Session) Detach(
	ch <-chan OutputFrame,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.detachLocked(ch)
}

// detachLocked removes the client whose send channel matches ch. Caller must hold s.mu.
func (s *Session) detachLocked(
	ch <-chan OutputFrame,
) {
	for cl := range s.clients {
		if ch == (<-chan OutputFrame)(cl.send) {
			delete(s.clients, cl)
			close(cl.send)
			return
		}
	}
}

// Write sends data to the PTY stdin. Returns an error if the PTY is not live.
func (s *Session) Write(
	data []byte,
) error {
	s.mu.Lock()
	ptmx := s.ptmx
	s.mu.Unlock()
	if ptmx == nil {
		return fmt.Errorf("session: write: session not live")
	}
	_, err := ptmx.Write(data)
	if err != nil {
		return fmt.Errorf("session: write: %w", err)
	}
	return nil
}

// SetTheme propagates the host terminal's light/dark theme to the session (spec: theme
// propagation). It does two decoupled things, both required to make a foreground app's
// automatic theme follow a Crowbar theme switch:
//
//  1. Sets the model's OSC 10/11 QUERY-answer colours (bg/fg) so an app that detects its
//     theme by querying the background colour reads the truth — this alone fixes a freshly
//     started app and any later query. It never re-emits colour to the transparent client
//     xterm (see model.SetDefaultColors), so the glass background is untouched.
//
//  2. If the foreground app subscribed to theme-change notifications (DEC private mode 2031),
//     injects a CSI ?997;n report into the PTY so an ALREADY-running app re-queries and
//     switches live. The report is gated on the 2031 subscription (a shell that never opted
//     in must not receive it) and deduped by polarity so only the first push and each
//     light<->dark flip notify.
//
// The model mutation runs under s.mu (via the §8.5 recover backstop, like every model
// access); the PTY write is issued OFF the lock through s.Write, so it can never run the
// blocking ptmx.Write while the session lock is held (the C1 invariant, spec §3.8).
func (s *Session) SetTheme(
	bg color.Color,
	fg color.Color,
	dark bool,
) {
	s.mu.Lock()
	ta, ok := s.model.(model.ThemeAware)
	if s.model == nil || !ok {
		s.mu.Unlock()
		return
	}
	var notifyEnabled bool
	s.mutateModelLocked(func() {
		ta.SetDefaultColors(bg, fg)
		notifyEnabled = ta.ThemeNotifyEnabled()
	})
	// Emit on the first subscribed push and on every polarity flip; skip a redundant
	// same-polarity push. themeEmitted/themeEmittedDark only advance when we actually emit,
	// so a push while unsubscribed (notifyEnabled==false) still emits the first time the app
	// later subscribes and pushes.
	emit := notifyEnabled && (!s.themeEmitted || s.themeEmittedDark != dark)
	if emit {
		s.themeEmitted = true
		s.themeEmittedDark = dark
	}
	s.mu.Unlock()

	if emit {
		_ = s.Write(themeNotifySeq(dark))
	}
}

// themeNotifySeq returns the DEC mode 2031 theme-change report a terminal sends to a
// subscribed app: CSI ?997;1n for a dark theme, CSI ?997;2n for a light theme.
func themeNotifySeq(dark bool) []byte {
	if dark {
		return []byte("\x1b[?997;1n")
	}
	return []byte("\x1b[?997;2n")
}

// Resize updates the PTY window size and reshapes the model in lockstep under one s.mu
// hold, with the syscall before the model reshape and no intervening model.Write, so the
// model and PTY never disagree on the active width (§4.2/§8.3). The model reshape is
// panic-isolated (§8.5). Returns an error if the PTY is not live.
func (s *Session) Resize(
	cols uint16,
	rows uint16,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ptmx == nil {
		return fmt.Errorf("session: resize: session not live")
	}
	if err := pty.Setsize(s.ptmx, &pty.Winsize{Cols: cols, Rows: rows}); err != nil {
		return fmt.Errorf("session: resize: %w", err)
	}
	s.mutateModelLocked(func() { s.model.Resize(int(cols), int(rows)) })
	if s.emitter != nil {
		// A resize can never be expressed as an absolute-addressed diff (the
		// grid dimensions themselves changed); force the next model-driven
		// frame to be a full keyframe.
		s.emitter.Invalidate()
	}
	s.dirty = true
	s.lastBlob = nil
	return nil
}

// Kill terminates the PTY process. For placeholder sessions (ptmx == nil) it only calls
// shutdown(). For live sessions it closes the PTY, kills the child, nils ptmx, then calls
// shutdown() WITHOUT holding s.mu (shutdown re-acquires it inside once.Do).
func (s *Session) Kill() {
	s.mu.Lock()
	if s.ptmx == nil {
		s.mu.Unlock()
		s.shutdown()
		return
	}
	_ = s.ptmx.Close()
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	s.ptmx = nil
	s.mu.Unlock()

	s.shutdown()
}

// pumpStep is the production critical section for one PTY output chunk. Under s.mu it
// either drives the model-driven path (model write FIRST, then a model-derived frame fans
// out — raw fan-out skipped entirely) or the raw path (RAW bytes fan out to live clients
// FIRST, zero added latency, §8.2, THEN the chunk feeds the model under a panic backstop),
// then — last, debounced — samples the foreground process group so neither the ioctl nor
// the app-death teardown can ever precede or delay the fan-out. OSC 7 is scanned outside the
// lock on the freshly-owned chunk.
func (s *Session) pumpStep(chunk []byte) {
	path, ok := parseLastOSC7(chunk)
	s.mu.Lock()
	defer s.mu.Unlock()
	if ok {
		s.cwd = path
	}
	if s.modelEmitHealthyLocked() { //nolint:nestif // healthy vs degraded emit paths with the documented per-chunk panic-fallback asymmetry; keeping it inline preserves the branch invariants.
		// Model-driven (spec §3.1): the model is written FIRST and clients
		// receive model-derived frames. Raw fan-out is skipped entirely —
		// UNLESS the emit path itself degrades on THIS chunk (see below).
		s.writeModelLocked(chunk)
		panicsBefore := s.modelPanics
		emittedNow := s.scheduleEmitLocked()
		if emittedNow && s.modelPanics > panicsBefore {
			// The model consumed this chunk (writeModelLocked succeeded) but the
			// emit/serialize path just panicked and recovered, so no frame went
			// out for it — without this fallback the chunk's visual delta would
			// be silently dropped until an unrelated resize/reattach keyframe.
			// Fan the raw bytes out so the update is not lost. This is an
			// approximation for a client whose screen is a model projection: it
			// is acceptable because (a) model and client were in sync as of the
			// last successful emit, and (b) any residual drift self-heals at the
			// next attach/resync keyframe. modelEmitHealthyLocked already logged
			// the degraded flip; from the NEXT chunk pumpStep takes the raw
			// branch below.
			//
			// Asymmetry (Task 7): this fallback only fires on the IMMEDIATE
			// emit path, where pumpStep still holds the triggering chunk. A
			// flip discovered inside the TRAILING TIMER callback (a burst
			// chunk that only armed a deferred emit) has no chunk in scope to
			// fall back with — that chunk's frame is lost. modelEmitHealthyLocked
			// still logs the degraded flip exactly once either way, and the
			// loss self-heals at the next attach/resync/resize keyframe, same
			// as the pre-existing residual-drift argument above; deferred
			// emission merely widens the window in which it can happen.
			s.fanOutLocked(chunk)
		}
	} else {
		// Raw path — §11.1 ordering preserved verbatim.
		s.fanOutLocked(chunk)
		s.writeModelLocked(chunk)
	}
	s.dirty = true
	if now := time.Now(); now.Sub(s.lastFgSampleAt) >= foregroundSampleInterval {
		s.lastFgSampleAt = now
		s.sampleForegroundLocked()
	}
}

// modelEmitHealthyLocked reports whether the session can still emit model-derived
// frames. Model-driven output is now the ONLY configured pipeline — the false
// (raw-streaming) branch is reachable solely via DEGRADATION, never configuration:
// a nil model (a placeholder before restore, no emitter) or a nonzero modelPanics
// can no longer be the source of truth, so the session flips to raw streaming for
// its remaining lifetime.
//
// modelPanics counts ONLY session-level model-access panics the §8.5 backstops
// recover: Resize (mutateModelLocked), Serialize (serializeLocked), Emit
// (emitLocked) and Prime (primeLocked). It does NOT count vtModel.Write's internal
// parse panics — those self-heal inside the model (recreateEmu blanks to a fresh
// emulator) and keep the session model-driven, so an adversarial byte stream that
// only trips the emulator's own parser never forces the raw fallback. Caller holds
// s.mu.
func (s *Session) modelEmitHealthyLocked() bool {
	if s.model == nil || s.emitter == nil {
		return false
	}
	if s.modelPanics == 0 {
		return true
	}
	if !s.modelDrivenFellBack {
		s.modelDrivenFellBack = true
		// One answerer at a time, always: once raw bytes (including device queries)
		// start reaching the client xterm, it becomes the answerer too — the model's
		// response sink must come down here or the app gets a double reply to every
		// query from now on (and, without this, recreateEmu would keep re-arming a
		// sink this session no longer wants after any later parse-panic recovery).
		if s.model != nil {
			s.model.SetResponseSink(nil)
		}
		_, _ = fmt.Fprintf(os.Stderr, "terminal: session %s: model degraded (parse panic), falling back to raw output\n", s.id)
	}
	return false
}

// scheduleEmitLocked implements the adaptive frame clock (spec §3.3, Task 7):
// when at least minEmitInterval has elapsed since the last emit, it emits
// immediately (so interactive echo is never batched); otherwise it arms — or
// leaves armed — a single trailing timer that fires exactly at the interval
// boundary, coalescing an arbitrarily long burst of chunks into one frame per
// interval. Caller holds s.mu. Returns whether it emitted synchronously
// (false means a trailing timer now owns the pending delta).
func (s *Session) scheduleEmitLocked() bool {
	if time.Since(s.lastEmitAt) >= minEmitInterval {
		s.lastEmitAt = time.Now()
		s.emitFrameLocked()
		return true
	}
	if s.emitTimer != nil {
		return false // trailing emit already armed
	}
	delay := minEmitInterval - time.Since(s.lastEmitAt)
	// Capture the timer's own identity so a stale, lock-blocked callback can
	// only clear ITS OWN handle: if an explicit stop (stopEmitTimerLocked) had
	// already nil'd s.emitTimer and a NEWER timer t2 was armed before this
	// callback got the lock, comparing against the captured local t (rather
	// than unconditionally nil-ing s.emitTimer) leaves t2's handle intact —
	// t2 still fires safely either way, but this keeps stopEmitTimerLocked
	// able to cancel it during the gap instead of losing track of it.
	var t *time.Timer
	t = time.AfterFunc(delay, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.emitTimer == t {
			s.emitTimer = nil
		}
		select {
		case <-s.done:
			return // session tore down while the timer was in flight
		default:
		}
		if !s.modelEmitHealthyLocked() {
			return
		}
		s.lastEmitAt = time.Now()
		// emitFrameLocked → emitLocked → DiffEmitter.Emit is safe to invoke
		// even if a newer immediate emit already flushed this same delta: Emit
		// is idempotent against its own primed base (an empty diff yields no
		// frame), so a harmless double-emit race here never double-delivers
		// visible output to clients.
		s.emitFrameLocked()
	})
	// Assign under the already-held s.mu (not inside the callback closure)
	// so the field update happens-before any concurrent read of s.emitTimer
	// under the lock; the callback only ever fires after AfterFunc returns
	// with delay > 0, but this ordering removes any reliance on that timing.
	s.emitTimer = t
	return false
}

// stopEmitTimerLocked cancels an armed trailing emit timer, if any, without
// running the emit it would otherwise have performed. Used at lifecycle
// boundaries (Resync, teardown) that either perform their own equivalent
// emit or no longer need one, so a stale timer can never fire a redundant or
// out-of-order frame afterward. Caller holds s.mu.
func (s *Session) stopEmitTimerLocked() {
	if s.emitTimer != nil {
		s.emitTimer.Stop()
		s.emitTimer = nil
	}
}

// flushPendingEmitLocked cancels an armed trailing emit timer and, only if
// one was armed (i.e. a delta is genuinely waiting), runs the emit
// immediately in its place. Used at boundaries — Attach's flush-then-serialize
// is the current caller — where client-visible state is about to be read or
// rebased and a delta accumulated under the trailing timer must reach
// existing clients first, rather than being silently superseded. A no-op
// when no timer is armed (the last emit was already synchronous). Caller
// holds s.mu.
func (s *Session) flushPendingEmitLocked() {
	if s.emitTimer == nil {
		return
	}
	s.stopEmitTimerLocked()
	s.lastEmitAt = time.Now()
	s.emitFrameLocked()
}

// emitFrameLocked derives one frame from the model and fans it out: a diff
// frame normally, a snapshot keyframe when the emitter demands one (see
// model.DiffEmitter.Emit for the canonical, exhaustive keyframe-trigger list).
// Caller holds s.mu. Reached either synchronously from
// scheduleEmitLocked/flushPendingEmitLocked or directly by Attach/Resync's own
// forced keyframes, which always reflect the full current model state regardless
// of the frame clock.
func (s *Session) emitFrameLocked() {
	data, needKeyframe := s.emitLocked()
	if needKeyframe {
		redraw := s.serializeLocked()
		if len(redraw) == 0 {
			return // serialize panicked → modelPanics bumped → raw fallback next chunk
		}
		s.fanOutFrameLocked(OutputFrame{SessionID: s.id, Data: redraw, Snapshot: true})
		s.primeLocked()
		return
	}
	if len(data) > 0 {
		s.fanOutFrameLocked(OutputFrame{SessionID: s.id, Data: data})
	}
}

// emitLocked / primeLocked wrap the emitter in the same §8.5 recover backstop
// as every other model access. A panic bumps modelPanics, flipping the session
// to raw fallback.
func (s *Session) emitLocked() (data []byte, needKeyframe bool) {
	defer func() {
		if r := recover(); r != nil {
			s.modelPanics++
			data, needKeyframe = nil, false
		}
	}()
	if s.emitForTest != nil {
		return s.emitForTest(s.model)
	}
	return s.emitter.Emit(s.model)
}

func (s *Session) primeLocked() {
	defer func() {
		if r := recover(); r != nil {
			s.modelPanics++
		}
	}()
	s.emitter.Prime(s.model)
}

// writeModelLocked feeds a chunk into the model under a recover backstop. In production
// this backstop is defence-in-depth: the real vtModel.Write recovers its own parse panics
// internally (recreateEmu) and never re-panics, so an adversarial byte stream self-heals in
// the model and does NOT bump s.modelPanics or force the raw fallback. The recover here only
// fires if a model.Write escapes a panic (a test fake, or a future backend without its own
// recover); when it does it bumps modelPanics and continues rather than stranding s.mu or
// killing the session (§8.2). Caller holds s.mu.
func (s *Session) writeModelLocked(chunk []byte) {
	defer func() {
		if r := recover(); r != nil {
			s.modelPanics++
		}
	}()
	if s.model != nil {
		s.model.Write(chunk)
	}
}

// serializeLocked runs serializer.Serialize under a recover so a Serialize/downcast panic
// can never escape (§8.5). Caller holds s.mu. On panic it bumps modelPanics and returns
// nil ("no redraw this time").
func (s *Session) serializeLocked() (redraw []byte) {
	defer func() {
		if r := recover(); r != nil {
			s.modelPanics++
			redraw = nil
		}
	}()
	return s.serializer.Serialize(s.model)
}

// mutateModelLocked runs a void model mutation under the same recover backstop as the
// Write/Serialize paths so a Resize-drain or teardown panic bumps modelPanics and returns
// instead of escaping (§8.5). Caller holds s.mu.
func (s *Session) mutateModelLocked(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			s.modelPanics++
		}
	}()
	fn()
}

// pump reads PTY stdout and delivers each chunk via pumpStep.
func (s *Session) pump() {
	defer safego.Recover("terminal.session.pump")
	defer s.shutdown()

	s.mu.Lock()
	ptmx := s.ptmx
	s.mu.Unlock()
	if ptmx == nil {
		return
	}

	buf := make([]byte, 64*1024)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			s.pumpStep(chunk)
		}
		if err != nil {
			if !isNormalPTYClose(err) {
				_, _ = fmt.Fprintf(os.Stderr, "terminal: session %s: pump error: %v\n", s.id, err)
			}
			return
		}
	}
}

// isNormalPTYClose reports whether err is the expected error when the shell exits.
// On Linux the PTY master returns EIO; on macOS it returns io.EOF.
func isNormalPTYClose(
	err error,
) bool {
	if errors.Is(err, io.EOF) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == syscall.EIO
	}
	return false
}

// fanOut delivers a chunk to all currently attached clients. Thin wrapper for callers that
// do not already hold s.mu.
func (s *Session) fanOut(
	chunk []byte,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fanOutLocked(chunk)
}

// fanOutLocked delivers a chunk to all currently attached clients. Clients whose channel is
// full are disconnected (drop-on-overflow). Caller must hold s.mu.
func (s *Session) fanOutLocked(
	chunk []byte,
) {
	s.fanOutFrameLocked(OutputFrame{SessionID: s.id, Data: chunk})
}

// fanOutFrameLocked delivers an already-built frame to all currently attached clients.
// Clients whose channel is full are disconnected (drop-on-overflow). Caller must hold s.mu.
func (s *Session) fanOutFrameLocked(
	frame OutputFrame,
) {
	var overflow []*client
	for cl := range s.clients {
		select {
		case cl.send <- frame:
		default:
			overflow = append(overflow, cl)
		}
	}

	for _, cl := range overflow {
		delete(s.clients, cl)
		close(cl.send)
	}
}

// shutdown reaps the child process, tears down the model, and closes the done + client
// channels exactly once. The cmd.Wait() here is the ONLY reap. once guards against a double
// Wait when both Kill() and pump()'s exit reach shutdown.
func (s *Session) shutdown() {
	s.once.Do(func() {
		code := -1
		if s.cmd != nil { //nolint:nestif // the single reap: Wait then classify exit vs ExitError to derive the code; shallow and self-contained.
			err := s.cmd.Wait()
			if err == nil {
				code = 0
			} else {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					code = exitErr.ExitCode()
				}
			}
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		// Stop the frame-clock timer (Task 7) under the same lock hold that
		// closes s.done, so a still-armed trailing emit can never fire after
		// teardown: its own s.done check would already guard against that,
		// but stopping it here also releases the timer goroutine promptly
		// instead of leaving it to wake up once more just to no-op.
		s.stopEmitTimerLocked()

		s.exitCode = code
		s.ptmx = nil
		if s.model != nil {
			s.model.Close()
		}
		for cl := range s.clients {
			close(cl.send)
		}
		s.clients = make(map[*client]struct{})
		close(s.done)
	})
}

// Snapshot returns the session's persisted blob and whether it changed since the last
// flush (§8.4). A model-less placeholder returns its stored rawBlob verbatim with no model
// access. A live session samples the foreground reset, reuses lastBlob when clean, otherwise
// serializes header+redraw under one s.mu hold and clears the dirty bit in that same hold.
func (s *Session) Snapshot() (blob []byte, changed bool) {
	if s.model == nil {
		return s.rawBlob, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkForegroundResetLocked()
	if !s.dirty && s.lastBlob != nil {
		return s.lastBlob, false
	}
	blob = append([]byte(s.header()), s.serializeLocked()...)
	s.lastBlob = blob
	s.dirty = false
	return blob, true
}

// header builds the mandatory CRWB1 size line, sourced entirely from the model's
// HeaderState so it never re-parses the stream (§12). Caller holds s.mu; only live sessions
// (model != nil) call it.
func (s *Session) header() string {
	cols, rows, alt, sb := s.model.HeaderState()
	altBit := 0
	if alt {
		altBit = 1
	}
	return fmt.Sprintf("CRWB1 %d %d %d %d\n", cols, rows, altBit, sb)
}

// InjectLocal feeds a clean-ANSI chunk into THIS session's model only — never the live wire
// and never the persisted .buf. It is the sole sanctioned way for the engine to push a
// synthetic, daemon-authored on-screen notice (restore/suspend) so it surfaces on the next
// Serialize (§12).
func (s *Session) InjectLocal(
	b []byte,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.injectLocalLocked(b)
}

// injectLocalLocked is the lock-free core of InjectLocal. Caller holds s.mu. It feeds bytes
// through writeModelLocked (the §8.5 recover) so a parse panic on a daemon notice cannot
// escape, and defensively drives the model to the primary buffer first so a notice can never
// land in a transient alt buffer (§12). It marks the session dirty and drops the cache; it
// never fans out.
func (s *Session) injectLocalLocked(
	b []byte,
) {
	if s.model == nil {
		return
	}
	if _, _, alt, _ := s.model.HeaderState(); alt {
		s.writeModelLocked([]byte("\x1b[?1049l\x1b[?47l\x1b[?1047l"))
	}
	s.writeModelLocked(b)
	s.dirty = true
	s.lastBlob = nil
}

// ForceSuspendSnapshot performs the §11.2 teardown and the suspending serialize as ONE
// uninterrupted s.mu critical section, so no live pumpStep chunk can interleave between the
// teardown and the serialize and repaint alt content onto the model just forced to primary.
// It drives the model to the primary buffer (OnForegroundReset), injects the notice into
// that primary screen, serializes a clean primary blob, caches it, and returns it for the
// engine to persist verbatim WITHOUT re-Snapshotting. Caller must NOT already hold s.mu.
func (s *Session) ForceSuspendSnapshot(
	notice []byte,
) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.model == nil {
		return s.rawBlob
	}
	s.mutateModelLocked(s.model.OnForegroundReset)
	s.injectLocalLocked(notice)
	blob := append([]byte(s.header()), s.serializeLocked()...)
	s.dirty = false
	s.lastBlob = blob
	return blob
}

// ExitCode returns the process exit code captured by shutdown(). -1 if not yet exited or
// killed with a signal (unknown code).
func (s *Session) ExitCode() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitCode
}

// BeginSuspendIfEligible atomically checks idle/no-clients/not-already-suspending and, if
// eligible, sets the suspending flag — closing the TOCTOU window before the kill.
func (s *Session) BeginSuspendIfEligible() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.clients) > 0 || s.suspending || !s.isIdleLocked() {
		return false
	}
	s.suspending = true
	return true
}

// Suspending reports whether the suspend flag has been set.
func (s *Session) Suspending() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.suspending
}

// BeginForceSuspend atomically sets the suspending flag for a DETACHED session even when it
// is not idle. Returns false if it has clients or is already suspending.
func (s *Session) BeginForceSuspend() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.clients) > 0 || s.suspending {
		return false
	}
	s.suspending = true
	return true
}

// MarkSuspendingForShutdown unconditionally sets suspending=true so reapOnDone preserves the
// .buf/meta row Shutdown wrote for daemon-restart restore.
func (s *Session) MarkSuspendingForShutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.suspending = true
}

// Health reports the session's parse-health for the engine's Stats observability surface
// (§9.4): whether the model is in the sticky degraded state, and the total recovered parse
// panics. The count combines the model's own ModelHealth.ParsePanics() (panics x/vt's Write
// recovered internally, self-healing without forcing the raw fallback) with the session-
// level s.modelPanics backstop counter (panics that escaped a model method —
// Resize/Serialize/Emit/Prime/teardown — into the §8.5 recover and DID flip the session to
// raw), so neither is write-only and a blanked-and-reparsed session is observable. A
// placeholder (model ==
// nil) or a backend that does not implement ModelHealth contributes only s.modelPanics.
func (s *Session) Health() (degraded bool, parsePanics int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	parsePanics = s.modelPanics
	if h, ok := s.model.(model.ModelHealth); ok {
		if h.Degraded() {
			degraded = true
		}
		parsePanics += h.ParsePanics()
	}
	return degraded, parsePanics
}

// ModelBytes returns the session's estimated resident size for the engine's memory ceiling:
// a placeholder counts only its rawBlob; a live session counts the model's grid+scrollback
// estimate plus the cached blob (§9.4) and the diff emitter's retained lastGrid estimate.
func (s *Session) ModelBytes() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.model == nil {
		return int64(len(s.rawBlob))
	}
	total := s.model.ModelBytes() + int64(len(s.lastBlob))
	if s.emitter != nil {
		// Stable-from-spawn estimate derived from the model's dims (see
		// model.EmitterGridBytes): counting the grid only after the first
		// Prime would make the session's reported size jump ~3× at its
		// first output, destabilizing the maintenance ceiling arithmetic.
		cols, rows, _, _ := s.model.HeaderState()
		total += model.EmitterGridBytes(cols, rows)
	}
	return total
}

// SerializedLen returns the byte length of a fresh serialize of the current screen (a
// placeholder reports its stored blob length). It is a PURE read: unlike Snapshot it does
// NOT consume the dirty bit or update the cached blob, so observers can poll the screen size
// without perturbing the cadence-flush change tracking.
func (s *Session) SerializedLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.model == nil {
		return len(s.rawBlob)
	}
	return len(s.serializeLocked())
}

// DropCachedBlob reclaims the live-session blob cache under memory pressure (§9.4 Phase-3
// pre-step): it nils lastBlob and marks the session dirty so the next Snapshot re-serializes
// a correct, current blob. No-op for a placeholder. Returns the bytes reclaimed.
func (s *Session) DropCachedBlob() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.model == nil || s.lastBlob == nil {
		return 0
	}
	n := int64(len(s.lastBlob))
	s.lastBlob = nil
	s.dirty = true
	return n
}

// parseLastOSC7 scans b for OSC 7 sequences of the form
//
//	ESC ] 7 ; file://[host]/path BEL
//
// and returns the decoded path from the last match found. Best-effort: partial sequences
// that span chunk boundaries are silently ignored.
func parseLastOSC7(b []byte) (string, bool) {
	prefix := []byte("\x1b]7;")
	last := ""
	found := false

	for {
		idx := bytes.Index(b, prefix)
		if idx < 0 {
			break
		}
		b = b[idx+len(prefix):]

		end := -1
		for i := 0; i < len(b); i++ {
			if b[i] == '\x07' {
				end = i
				break
			}
			if b[i] == '\x1b' && i+1 < len(b) && b[i+1] == '\\' {
				end = i
				break
			}
		}
		if end < 0 {
			break
		}

		uri := string(b[:end])
		b = b[end+1:]

		if !strings.HasPrefix(uri, "file://") {
			continue
		}

		parsed, err := url.Parse(uri)
		if err != nil || parsed.Path == "" {
			continue
		}

		last = parsed.Path
		found = true
	}

	return last, found
}
