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
	return o
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
	begun    ledgerView
	led      *ledger
	run      *runState
	halted   atomic.Pointer[Halt]
	progress atomic.Pointer[Progress]
}

// Begin takes the home lock, reads the layout and the ledger, and closes a run
// a crash left open. It returns an error, and no session, only when the home is
// held by another process (ErrHomeLocked) or cannot be cleaned or written (an
// I/O failure); the lock is released. A home it must not migrate, because LAYOUT
// is newer than the catalog or unreadable, or the ledger is corrupt or
// unreadable, is no error: the session is live, holds the lock, and its Halt
// says why.
func Begin(
	_ context.Context,
	h home.Home,
	cat *Catalog,
	opts Options,
) (*Session, error) {
	lock, err := lockHome(h)
	if err != nil {
		return nil, err
	}
	s := &Session{home: h, cat: cat, opts: opts.withDefaults(), lock: lock}
	if err := s.load(); err != nil {
		return nil, errors.Join(fmt.Errorf("migrate: begin: %w", err), s.Close())
	}
	return s, nil
}

// load reads the home without changing it, until it knows the home may be
// migrated: a refused home must stay byte-identical, since an older binary
// must not write into a newer home.
func (s *Session) load() error {
	read := readHome(s.home, s.cat)
	s.layout = read.layout
	if read.refusal != nil {
		s.refuse(read.refusal.reason, read.refusal.message)
		return nil
	}
	s.begun = read.view
	if err := cleanPartialBackups(s.home); err != nil {
		return err
	}
	return s.closeDanglingRun()
}

// closeDanglingRun ends the run a killed process left open, so the ledger never
// holds two live runs. It is the only ledger write Begin makes, and only when a
// run is dangling.
func (s *Session) closeDanglingRun() error {
	start, ok := s.begun.danglingRun()
	if !ok {
		return nil
	}
	outcome, detail := outcomeInterrupted, "the process stopped before the run ended"
	if s.leftNothingUndone(start) {
		outcome, detail = outcomeCompleted, ""
	}
	if _, err := s.openForAppend(); err != nil {
		return fmt.Errorf("close unfinished run %s: open ledger: %w", start.Run, err)
	}
	if err := s.writeRunEnd(start.Run, nil, outcome, detail); err != nil {
		return fmt.Errorf("close unfinished run %s: %w", start.Run, err)
	}
	s.opts.Log.Info("migrate: closed a run that never ended", "run", start.Run, "outcome", outcome)
	return nil
}

// leftNothingUndone is whether every step the run set out to do is done in this
// home, so only its closing record was lost. A step the catalog dropped cannot
// be vouched for.
func (s *Session) leftNothingUndone(
	start record,
) bool {
	left := stepNumbers(s.pending())
	return !slices.ContainsFunc(start.Pending, func(n int) bool {
		return !s.cat.has(n) || slices.Contains(left, n)
	})
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
	return s.cat.pending(s.layout, s.view())
}

// pending lists the steps a home at layout still has to run, in order. A bump
// step is pending exactly while LAYOUT sits at its From; every other step is
// pending until the ledger says it was applied.
func (c *Catalog) pending(
	layout int,
	v ledgerView,
) []Step {
	return c.order(layout, func(s Step) bool {
		return !s.bumps() && v.applied(s.Number, s.Name)
	})
}
