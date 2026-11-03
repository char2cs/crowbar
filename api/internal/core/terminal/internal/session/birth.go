package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/color"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/creack/pty"

	"github.com/char2cs/crowbar/api/internal/core/safego"
	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

// newModel is a seam so a test can inject a panicking model to reach the session's recover
// backstops (vtModel recovers its own panics). Production never reassigns it; sessions capture it
// at construction so a test restoring it cannot race a live session.
var newModel = model.New

// defaultScrollbackLines is the scrollback depth a create/restore with no explicit
// value resolves to — mirroring the frontend's terminalScrollback default (§9.1).
const defaultScrollbackLines = 10000

// responseReplyBufDepth bounds the device-query reply queue between the model's sink and the
// blocking ptmx.Write: when it and the PTY input fill, replies drop rather than wedge the lock.
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

// spawnParams carries one birth mode: create (Blob == nil, size from Cols/Rows/ScrollbackLines) or
// restore (size from the blob's CRWB1 header). ThemeBg/ThemeFg apply to both.
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

// WithTheme seeds the host's default colours (the OSC 10/11 query answers) at birth, before the pump
// runs: a CLI queries its background within milliseconds of exec, before any SetTheme call could land.
// A nil channel leaves the emulator's own default, matching model.SetDefaultColors.
func WithTheme(
	bg color.Color,
	fg color.Color,
) Option {
	return func(p *spawnParams) {
		p.ThemeBg, p.ThemeFg = bg, fg
	}
}

// applyBirthTheme installs the host colours between newModel and `go s.pump()`, before any query can
// arrive. A model without ThemeAware keeps its own defaults.
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

// New spawns a PTY at cols×rows, builds the model at that size and scrollback, and starts the pump.
// Clients get model-derived frames; raw streaming is only the degraded fallback.
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

// NewRestored spawns a PTY sized from the blob's CRWB1 header before the first read, and replays the
// redraw into the fresh model before the pump starts, reproducing the screen exactly.
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

// spawn is the one PTY-birth helper: it sizes the PTY before the first read (model==PTY invariant),
// builds the model, replays any restore, and starts the pump. The child lives as long as the
// session, not ctx: context.WithoutCancel keeps ctx's values but drops its cancellation.
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

	// Size the PTY before any Read so the shell's first output has the right width. A PTY
	// that cannot be sized is abandoned here (killed, reaped): no session owns the child yet.
	if err := pty.Setsize(ptmx, &pty.Winsize{Cols: winDim(cols), Rows: winDim(rows)}); err != nil {
		closePTY(ptmx)
		killErr := killSignal(cmd.Process)
		// A child that could not be killed is not waited on: reap would block.
		if killErr == nil {
			reap(cmd)
		}
		return errors.Join(fmt.Errorf("session: size pty: %w", err), killErr)
	}

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
// agentic engine to launch vendor CLIs with descriptor-built args
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

// startResponseSink keeps the blocking ptmx.Write off the model's drain goroutine: an app spamming
// queries without reading stdin would otherwise stall the drain under s.mu and wedge the engine.
// The sink only sends non-blocking to a bounded queue; replyCh is never closed so late sends drop.
func (s *Session) startResponseSink(
	ptmx *os.File,
) {
	replyCh := make(chan []byte, responseReplyBufDepth)
	done := s.done
	safego.Go("terminal.session.replyWriter", func() {
		for {
			select {
			case reply := <-replyCh:
				// A failing write means the PTY is going away: log it and keep
				// draining replyCh until s.done releases the goroutine.
				if _, err := ptmx.Write(reply); err != nil {
					slog.Warn("terminal: device-query reply not delivered", "session", s.id, "err", err)
				}
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

// winDim narrows a resolved dimension to pty.Winsize's uint16. The resolvers already clamp, but the
// conversion itself must be provably in range: a client may ask for 70000 columns.
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
