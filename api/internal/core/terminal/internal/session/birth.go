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

var newModel = model.New

const defaultScrollbackLines = 10000

const responseReplyBufDepth = 64

func newBareSession(
	id string,
	shell string,
	cwd string,
	profileID string,
) *Session {
	s := &Session{
		id:         id,
		clients:    make(map[*client]struct{}),
		done:       make(chan struct{}),
		cwd:        cwd,
		shell:      shell,
		profileID:  profileID,
		exitCode:   -1,
		emitter:    model.NewDiffEmitter(),
		newModel:   newModel,
		now:        time.Now,
		pumpNotify: make(chan struct{}, 1),
	}
	s.sampleForegroundLocked = s.checkForegroundResetLocked
	return s
}

type spawnParams struct {
	Cols            int
	Rows            int
	ScrollbackLines int
	Blob            []byte
	ThemeBg         color.Color
	ThemeFg         color.Color
}

type Option func(*spawnParams)

func WithTheme(
	bg color.Color,
	fg color.Color,
) Option {
	return func(p *spawnParams) {
		p.ThemeBg, p.ThemeFg = bg, fg
	}
}

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

func (s *Session) spawn(
	ctx context.Context,
	name string,
	args []string,
	env []string,
	p spawnParams,
) error {
	cols, rows, sbLines, redraw := s.resolveBirth(p)

	cmd := exec.CommandContext(context.WithoutCancel(ctx), name, args...) //nolint:gosec // G204: see above.
	cmd.Dir = s.cwd
	cmd.Env = env

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("session: pty start: %w", err)
	}

	if err := pty.Setsize(ptmx, &pty.Winsize{Cols: winDim(cols), Rows: winDim(rows)}); err != nil {
		closePTY(ptmx)
		killErr := killSignal(cmd.Process)
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
	if len(redraw) == 0 {
		s.latchForeground()
	}
	s.startResponseSink(s.ptmx)

	go s.pump()
	return nil
}

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

func (s *Session) startResponseSink(
	ptmx *os.File,
) {
	replyCh := make(chan []byte, responseReplyBufDepth)
	done := s.done
	safego.Go("terminal.session.replyWriter", func() {
		for {
			select {
			case reply := <-replyCh:
				if _, err := ptmx.Write(reply); err != nil {
					slog.Warn("terminal: device-query reply not delivered", "session", s.id, "err", err)
				}
			case <-done:
				return
			}
		}
	})
	s.replySink = func(reply []byte) {
		select {
		case replyCh <- reply:
		default:
		}
	}
	s.model.SetResponseSink(s.replySink)
}

func (s *Session) resolveBirth(
	p spawnParams,
) (cols, rows, sbLines int, redraw []byte) {
	if p.Blob != nil {
		hc, hr, hsb, body := parseBlob(p.Blob)
		return resolveCols(hc), resolveRows(hr), resolveScrollback(hsb), body
	}
	return resolveCols(p.Cols), resolveRows(p.Rows), resolveScrollback(p.ScrollbackLines), nil
}

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
