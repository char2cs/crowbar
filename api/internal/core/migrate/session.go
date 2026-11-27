package migrate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"slices"
	"sync/atomic"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
	"github.com/char2cs/crowbar/api/internal/core/home"
)

type Options struct {
	Version   string
	Clock     func() time.Time
	FreeBytes FreeBytesFunc
	HaveGit   func() bool
	Log       *slog.Logger
	Explain   func(error) (string, bool)
}

func (o Options) withDefaults() Options {
	if o.Clock == nil {
		o.Clock = time.Now
	}
	if o.FreeBytes == nil {
		o.FreeBytes = freeBytes
	}
	if o.HaveGit == nil {
		o.HaveGit = gitOnPath
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.Explain == nil {
		o.Explain = func(error) (string, bool) { return "", false }
	}
	return o
}

func (o Options) plain(err error) error {
	if words, ok := o.Explain(err); ok {
		return errors.New(words)
	}
	return err
}

func gitOnPath() bool {
	_, err := exec.LookPath(binpath.Resolve("git"))
	return err == nil
}

type Progress struct {
	Phase Phase
	Step  ID
	Index int
	Total int
}

type PostOpenRunner interface {
	RunPostOpen(ctx context.Context, impls Impls) error
}

var _ PostOpenRunner = (*Session)(nil)

type Session struct {
	home     home.Home
	cat      *Catalog
	opts     Options
	lock     *homeLock
	layout   int
	begun    ledgerView
	led      *ledger
	run      *runState
	again    map[int]bool
	halted   atomic.Pointer[Halt]
	progress atomic.Pointer[Progress]
}

func Begin(
	ctx context.Context,
	h home.Home,
	cat *Catalog,
	opts Options,
) (*Session, error) {
	if err := createHome(h); err != nil {
		return nil, err
	}
	s := &Session{home: h, cat: cat, opts: opts.withDefaults()}
	lock, err := lockHome(h)
	switch {
	case errors.Is(err, ErrHomeLocked):
		return nil, err
	case err != nil:
		s.refuse(HaltIOFailed, s.opts.plain(err).Error())
	default:
		s.lock = lock
		s.load(ctx)
	}
	return s, nil
}

func (s *Session) load(
	ctx context.Context,
) {
	read := readHome(s.home, s.cat)
	s.layout = read.layout
	if read.refusal != nil {
		s.refuse(read.refusal.reason, read.refusal.message)
		return
	}
	s.begun = read.view
	if err := s.tidy(); err != nil {
		s.refuse(HaltIOFailed, s.opts.plain(err).Error())
		return
	}
	s.again = s.cat.recurring(ctx, s.env(), s.layout, s.begun)
}

func (s *Session) tidy() error {
	if err := cleanPartialBackups(s.home); err != nil {
		return err
	}
	if err := s.closeDanglingRun(); err != nil {
		return err
	}
	return pruneBackups(s.home, s.view(), s.opts.Log)
}

func (s *Session) env() Env {
	return Env{Home: s.home, Log: s.opts.Log, History: History{home: s.home, view: s.view()}}
}

func (s *Session) closeDanglingRun() error {
	end, ok := s.cat.endOfDangling(s.layout, s.begun)
	if !ok {
		return nil
	}
	if _, err := s.openForAppend(); err != nil {
		return fmt.Errorf("close unfinished run %s: open ledger: %w", end.run, err)
	}
	if err := s.writeRunEnd(end.run, nil, end.outcome, end.detail); err != nil {
		return fmt.Errorf("close unfinished run %s: %w", end.run, err)
	}
	s.opts.Log.Info("migrate: closed a run that never ended", "run", end.run, "outcome", end.outcome)
	return nil
}

type runEnd struct {
	run     string
	outcome string
	detail  string
}

func (e runEnd) record(
	at time.Time,
	layout int,
	counts Counts,
) record {
	return record{Kind: kindRunEnd, At: at, Run: e.run, To: &layout, Counts: counts, Outcome: e.outcome, Error: e.detail}
}

func (c *Catalog) endOfDangling(
	layout int,
	v ledgerView,
) (runEnd, bool) {
	start, ok := v.danglingRun()
	if !ok {
		return runEnd{}, false
	}
	left := stepNumbers(c.pending(layout, v, nil))
	undone := slices.ContainsFunc(start.Pending, func(n int) bool {
		s, known := c.step(n)
		return !known || slices.Contains(left, n) || s.Recurs && !v.doneIn(start.Run, n)
	})
	if undone {
		return runEnd{start.Run, outcomeInterrupted, "the process stopped before the run ended"}, true
	}
	return runEnd{start.Run, outcomeCompleted, ""}, true
}

func (c *Catalog) settled(
	layout int,
	v ledgerView,
) ledgerView {
	if end, ok := c.endOfDangling(layout, v); ok {
		return v.ended(end.record(time.Time{}, layout, nil))
	}
	return v
}

func (s *Session) Halt() *Halt {
	return s.halted.Load()
}

func (s *Session) Progress() Progress {
	if p := s.progress.Load(); p != nil {
		return *p
	}
	return Progress{}
}

func (s *Session) Close() error {
	var ledgerErr, lockErr error
	if s.led != nil {
		ledgerErr = s.led.Close()
	}
	if s.lock != nil {
		lockErr = s.lock.Close()
	}
	return errors.Join(ledgerErr, lockErr)
}

func (s *Session) view() ledgerView {
	if s.led != nil {
		return s.led.view
	}
	return s.begun
}

func (s *Session) openForAppend() (*ledger, error) {
	if s.led != nil {
		return s.led, nil
	}
	l, err := openLedger(ledgerPath(s.home))
	if err != nil {
		return nil, err
	}
	s.led = l
	return l, nil
}

func (s *Session) now() time.Time {
	return s.opts.Clock().UTC()
}

func (s *Session) pending() []Step {
	return s.cat.pending(s.layout, s.view(), s.again)
}

func (c *Catalog) pending(
	layout int,
	v ledgerView,
	again map[int]bool,
) []Step {
	return c.order(layout, func(s Step) bool {
		return !s.bumps() && v.applied(s.Number, s.Name) && !again[s.Number]
	})
}

func (c *Catalog) recurring(
	ctx context.Context,
	env Env,
	layout int,
	v ledgerView,
) map[int]bool {
	again := map[int]bool{}
	for _, s := range c.steps {
		if !s.Recurs || s.From < layout || !v.applied(s.Number, s.Name) {
			continue
		}
		env.Earlier, env.Notes = v.checkpoints(s.Number, s.Name), v.notes(s.Number, s.Name)
		plan, err := s.Plan(ctx, env)
		if err != nil {
			env.Log.WarnContext(ctx, "migrate: a step that runs again could not be planned; it is skipped this boot",
				"step", s.Number, "name", s.Name, "err", err)
			continue
		}
		if !plan.Empty() {
			again[s.Number] = true
		}
	}
	return again
}
