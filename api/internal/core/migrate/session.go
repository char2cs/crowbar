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

// Options configure a Session; every field but Version has a working default.
type Options struct {
	// Version is the binary version written to each run's first record.
	Version string
	// Clock stamps records and names runs; default time.Now.
	Clock func() time.Time
	// FreeBytes probes disk space; default is the operating system's.
	FreeBytes FreeBytesFunc
	// HaveGit reports whether git can be run; default looks it up on PATH.
	HaveGit func() bool
	// Log receives the session's progress; default slog.Default().
	Log *slog.Logger
	// Explain names a step's failure in plain words for the halt, when it can;
	// the technical text stays in the log and the ledger. Default: it never can.
	Explain func(error) (string, bool)
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

// plain is err as Explain names it, or err itself when Explain cannot.
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

// Progress says which step a session is on. Index counts from 1 across the
// whole run, pre-open and post-open together, out of Total.
type Progress struct {
	Phase Phase
	Step  ID
	Index int
	Total int
}

// PostOpenRunner runs the post-open steps once the databases are open.
type PostOpenRunner interface {
	RunPostOpen(ctx context.Context, impls Impls) error
}

var _ PostOpenRunner = (*Session)(nil)

// Session owns the home for one boot: it holds the home lock, knows the layout,
// and runs the catalog's pending steps in order, recording each run. It is not
// safe for concurrent use, except Halt and Progress.
type Session struct {
	home   home.Home
	cat    *Catalog
	opts   Options
	lock   *homeLock
	layout int
	// begun is the ledger as Begin read it. The file is opened, and led set, by
	// the first record a run writes, so a boot with nothing to do creates nothing.
	begun ledgerView
	led   *ledger
	run   *runState
	// again is the applied recurring steps this boot runs once more.
	again    map[int]bool
	halted   atomic.Pointer[Halt]
	progress atomic.Pointer[Progress]
}

// Begin takes the home lock, reads the layout and the ledger, and closes a run
// a crash left open. A session comes back unless the home cannot be created or
// is held (ErrHomeLocked); any other refusal is its Halt, with no lock if that failed.
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

// load reads the home without changing it, until it knows the home may be
// migrated: a refused home must stay byte-identical, since an older binary
// must not write into a newer home.
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

// tidy undoes what a killed process left, half-written backups and a run with no
// end, then deletes the backups a completed run superseded. These are the only
// writes Begin makes.
func (s *Session) tidy() error {
	if err := cleanPartialBackups(s.home); err != nil {
		return err
	}
	if err := s.closeDanglingRun(); err != nil {
		return err
	}
	return pruneBackups(s.home, s.view(), s.opts.Log)
}

// env is what a step is handed: the home, the log and the ledger as it stands.
func (s *Session) env() Env {
	return Env{Home: s.home, Log: s.opts.Log, History: History{home: s.home, view: s.view()}}
}

// closeDanglingRun ends the run a killed process left open, so the ledger never
// holds two live runs. It is the only ledger write Begin makes, and only when a
// run is dangling.
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

// runEnd is how a run ends: the record that closes it.
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

// endOfDangling ends a run a killed process left open: completed if every step it set
// out to do is done, else interrupted. A dropped step cannot be vouched for, and a
// recurring step counts only if this run says it finished.
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

// settled is the ledger as it stands once Begin has ended a dangling run; a
// preview reads it so that it sees what the boot will.
func (c *Catalog) settled(
	layout int,
	v ledgerView,
) ledgerView {
	if end, ok := c.endOfDangling(layout, v); ok {
		return v.ended(end.record(time.Time{}, layout, nil))
	}
	return v
}

// Halt is why the boot must not go on, or nil while it may. It is safe to call
// from another goroutine, and the Halt it returns is never changed.
func (s *Session) Halt() *Halt {
	return s.halted.Load()
}

// Progress is the step being run now, or the last one run. It is safe to call
// from another goroutine.
func (s *Session) Progress() Progress {
	if p := s.progress.Load(); p != nil {
		return *p
	}
	return Progress{}
}

// Close releases the ledger file and the home lock; it writes nothing and may
// be called again.
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

// view is the ledger as it stands, whether or not the file is open yet.
func (s *Session) view() ledgerView {
	if s.led != nil {
		return s.led.view
	}
	return s.begun
}

// openForAppend opens the ledger the first time a run needs to write to it.
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

// pending lists the steps a home at layout still has to run, in order. A bump
// step is pending exactly while LAYOUT sits at its From; every other step is
// pending until the ledger says it was applied, or while it is in again.
func (c *Catalog) pending(
	layout int,
	v ledgerView,
	again map[int]bool,
) []Step {
	return c.order(layout, func(s Step) bool {
		return !s.bumps() && v.applied(s.Number, s.Name) && !again[s.Number]
	})
}

// recurring is the applied recurring steps a home at layout must run again:
// those whose Plan, given what their earlier attempts recorded, finds work. A
// plan that fails skips its step with a warning: the home runs on what it did.
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
