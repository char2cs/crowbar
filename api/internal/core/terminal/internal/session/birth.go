package session

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"math"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/creack/pty"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

// newModel is the model-construction seam. It is a package-level var only so a unit test can
// substitute a model whose Resize/Serialize panics, driving the §8.5 session recover
// backstops (mutateModelLocked/serializeLocked) through Session.Resize/Attach — the real
// vtModel recovers Write panics internally, so those session backstops are otherwise
// unreachable from a test. Production never reassigns it. A session captures it once at
// construction (Session.newModel), so its own goroutines never read the package var — a
// test restoring the seam can't race a session that is still rebuilding its model.
var newModel = model.New

// defaultScrollbackLines is the scrollback depth a create/restore with no explicit
// value resolves to — mirroring the frontend's terminalScrollback default (§9.1).
const defaultScrollbackLines = 10000

// responseReplyBufDepth bounds the device-query reply queue that decouples the
// model's response sink from the blocking ptmx.Write (spec §3.8, C1). A hostile
// or broken foreground app can emit queries faster than it drains its own stdin;
// once the queue and the PTY input buffer are both full, further replies are
// dropped (a lost query answer times out, it never wedges the session lock).
const responseReplyBufDepth = 64

// newBareSession allocates a Session shell with no PTY and no model. New/NewRestored/
// NewCommand fill it in via spawn.
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
		emitter:   model.NewDiffEmitter(),
		newModel:  newModel,
		now:       time.Now,
		// 1-buffered: notifyPumpLocked's send is non-blocking, so this is a coalescing
		// edge, not a queue. Always allocated (a nil channel would make the send's
		// select fall through to default forever, silently disabling the seam).
		pumpNotify: make(chan struct{}, 1),
	}
	s.sampleForegroundLocked = s.checkForegroundResetLocked
	return s
}

// spawnParams carries exactly one of the two birth modes (§9.1): create (Blob == nil,
// size+scrollback from Cols/Rows/ScrollbackLines) or restore (Blob != nil, size+scrollback
// parsed from the blob's CRWB1 header, the rest ignored).
//
// ThemeBg/ThemeFg are orthogonal to that choice and apply to both: they are the host
// terminal's default colours, seeded via WithTheme.
type spawnParams struct {
	Cols            int
	Rows            int
	ScrollbackLines int
	Blob            []byte
	ThemeBg         color.Color
	ThemeFg         color.Color
}

// Option customises a session's birth. It exists so the host theme can be threaded into
// the three constructors without every call site that has no theme to give (every test,
// and any future backend) having to pass a pair of nils.
type Option func(*spawnParams)

// WithTheme seeds the host terminal's default background/foreground — the colours an
// OSC 10/11 QUERY answers with — into the session's model AT BIRTH, before the io pump
// goroutine exists to feed it a single byte.
//
// That ordering is the whole point, and it is why this is a constructor option rather than
// a post-construction call. A vendor CLI that detects light/dark by querying the background
// (codex 0.146.0, Claude Code's `auto`) asks within milliseconds of exec, and Session.SetTheme
// cannot beat it: the process is already running by the time any caller holds the *Session.
// Seeding here closes the window by construction instead of narrowing it — the model is
// already answering with the truth before it can be asked.
//
// A nil channel is a no-op for that channel, matching model.SetDefaultColors: an unknown
// host theme must leave the emulator's own default in place, never reset it.
func WithTheme(
	bg color.Color,
	fg color.Color,
) Option {
	return func(p *spawnParams) {
		p.ThemeBg, p.ThemeFg = bg, fg
	}
}

// applyBirthTheme installs the host's default colours on a freshly built model. Called from
// spawn between newModel and `go s.pump()`, so no PTY byte — and therefore no query
// — can have reached the model yet. Guarded like every other optional-interface access
// (ModelHealth, ThemeAware in Session.SetTheme): a backend or test model that implements
// neither simply keeps its own defaults.
func applyBirthTheme(
	m model.TerminalModel,
	p spawnParams,
) {
	if m == nil || (p.ThemeBg == nil && p.ThemeFg == nil) {
		return
	}
	if ta, ok := m.(model.ThemeAware); ok {
		ta.SetDefaultColors(p.ThemeBg, p.ThemeFg)
	}
}

// New spawns a PTY subprocess at cols×rows, builds the screen model at that size and the
// resolved scrollback depth, and starts the io pump. Every session spawned this way is
// model-driven (spec 2026-07-03): clients receive model-derived frames, and raw streaming
// survives only as the degraded fallback.
func New(
	ctx context.Context,
	id string,
	shell string,
	cwd string,
	profileID string,
	env []string,
	cols int,
	rows int,
	scrollbackLines int,
	opts ...Option,
) (*Session, error) {
	s := newBareSession(id, shell, cwd, profileID)
	p := spawnParams{Cols: cols, Rows: rows, ScrollbackLines: scrollbackLines}
	for _, o := range opts {
		o(&p)
	}
	if err := s.spawn(ctx, shell, nil, env, p); err != nil {
		return nil, err
	}
	return s, nil
}

// NewRestored spawns a PTY subprocess and rebuilds the screen model from a persisted blob:
// spawn parses the blob's CRWB1 header for the authoritative size+scrollback (§12), sizes
// the PTY to it BEFORE the first read, and feeds the redraw bytes into the fresh model
// before the pump starts so the restored screen is reproduced exactly.
func NewRestored(
	ctx context.Context,
	id string,
	shell string,
	cwd string,
	profileID string,
	env []string,
	rawBlob []byte,
	opts ...Option,
) (*Session, error) {
	s := newBareSession(id, shell, cwd, profileID)
	p := spawnParams{Blob: rawBlob}
	for _, o := range opts {
		o(&p)
	}
	if err := s.spawn(ctx, shell, nil, env, p); err != nil {
		return nil, err
	}
	return s, nil
}

// spawn is the single PTY-birth helper shared by the create, restore and command paths. It
// starts name+args under a PTY, sizes it to the resolved dimensions BEFORE the first read
// (preserving the model==PTY size invariant, §4.2), builds the model+serializer at that
// size, replays the restore redraw (if any) into the model, and launches the pump goroutine.
//
// The child is bound to the SESSION's lifetime — it ends when Kill takes its process group
// or it exits on its own — never to ctx's: a PTY outlives the request that created it by
// design, so ctx contributes its values but context.WithoutCancel drops its cancellation.
// (With no Done channel, exec also starts no watcher goroutine for it.)
func (s *Session) spawn(
	ctx context.Context,
	name string,
	args []string,
	env []string,
	p spawnParams,
) error {
	cols, rows, sbLines, redraw := s.resolveBirth(p)

	// name is the operator-configured login shell or agent command (resolved through
	// binpath), not attacker-controlled; spawning it is the whole point of a session.
	cmd := exec.CommandContext(context.WithoutCancel(ctx), name, args...) //nolint:gosec // G204: see above.
	cmd.Dir = s.cwd
	cmd.Env = env

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("session: pty start: %w", err)
	}

	// Size the PTY to the persisted/requested dimensions before any Read so the shell's
	// first output is generated at the correct width.
	_ = pty.Setsize(ptmx, &pty.Winsize{Cols: winDim(cols), Rows: winDim(rows)})

	m, ser := s.newModel(cols, rows, sbLines)
	if len(redraw) > 0 {
		m.Write(redraw)
	}
	applyBirthTheme(m, p)

	s.ptmx = ptmx
	s.cmd = cmd
	s.model = m
	s.serializer = ser
	s.cols, s.rows, s.scrollback = cols, rows, sbLines
	s.startResponseSink(s.ptmx)

	go s.pump()
	return nil
}

// NewCommand spawns an explicit argv (not a login shell) under a PTY. Used by the
// agentic engine to launch vendor CLIs (claude/codex) with descriptor-built args
// and env. The joined argv is stored as the display "shell".
func NewCommand(
	ctx context.Context,
	id string,
	argv []string,
	cwd string,
	env []string,
	cols int,
	rows int,
	scrollbackLines int,
	opts ...Option,
) (*Session, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("session: NewCommand requires non-empty argv")
	}
	s := newBareSession(id, strings.Join(argv, " "), cwd, "")
	s.command = true
	p := spawnParams{Cols: cols, Rows: rows, ScrollbackLines: scrollbackLines}
	for _, o := range opts {
		o(&p)
	}
	if err := s.spawn(ctx, argv[0], argv[1:], env, p); err != nil {
		return nil, err
	}
	return s, nil
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
	s.replySink = func(reply []byte) {
		// reply is a fresh per-call allocation from vtEmu.drainResponses
		// (append([]byte(nil), buf[:n]...)), so we own it outright — no copy
		// needed before handing it to the writer goroutine.
		select {
		case replyCh <- reply:
		default:
			// Queue full and the PTY input buffer is backed up too; drop the
			// reply rather than block the drain goroutine (see the doc above).
		}
	}
	s.model.SetResponseSink(s.replySink)
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

// winDim narrows a resolved dimension to the uint16 a pty.Winsize field is.
//
// The resolvers already clamp, so the ceiling here is belt-and-braces — but it is
// the conversion that has to be provably in range, not the value that reached it,
// and a client is free to ask for 70000 columns.
func winDim(n int) uint16 {
	if n < 1 {
		return 1
	}
	if n > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(n)
}

func resolveCols(
	c int,
) int {
	if c <= 0 {
		return 80
	}
	return min(c, math.MaxUint16)
}

func resolveRows(
	r int,
) int {
	if r <= 0 {
		return 24
	}
	return min(r, math.MaxUint16)
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
