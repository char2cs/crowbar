package migrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

// StepPlan is a pending step with what it would do.
type StepPlan struct {
	Step Step
	Plan Plan
}

// BackupPlan is what the run does about state/ first. A copy is Required, and
// weighs Bytes with FreeBytes to spare; Kept names the run whose backup of the
// state a retry keeps instead of copying. Neither means there is nothing to do.
type BackupPlan struct {
	Required  bool
	Bytes     int64
	FreeBytes uint64
	Kept      string
}

// Report is what booting this binary on a home would do, read without
// changing the home.
type Report struct {
	Home          string
	Layout, Known int
	LockHeld      bool
	// Steps are in execution order: what a session would run.
	Steps  []StepPlan
	Backup BackupPlan
	// Last is the newest run the ledger records, nil while it records none.
	Last *RunSummary
}

// Inspect previews the migration of h against cat without changing h: no lock
// beyond a momentary probe, no repair, nothing created. A home or step the boot
// would halt on comes back as the *Halt the boot would set.
func Inspect(
	ctx context.Context,
	h home.Home,
	cat *Catalog,
	opts Options,
) (Report, error) {
	opts = opts.withDefaults()
	read := readHome(h, cat)
	if read.refusal != nil {
		return Report{}, haltAt(h, cat, read.layout, read.refusal.reason, read.refusal.message)
	}
	pending := cat.pending(read.layout, read.view)
	res, err := checkPending(ctx, Env{Home: h, Log: opts.Log}, pending, read.view, opts)
	var f *fault
	if errors.As(err, &f) {
		return Report{}, haltAt(h, cat, read.layout, f.reason, f.Error()).at(f.step)
	}
	if err != nil {
		return Report{}, err
	}
	steps := make([]StepPlan, 0, len(pending))
	for i, step := range pending {
		steps = append(steps, StepPlan{Step: step, Plan: res.plans[i]})
	}
	due := res.backup
	report := Report{
		Home: h.Root(), Layout: read.layout, Known: cat.Known(),
		LockHeld: probeLock(h), Steps: steps,
		Backup: BackupPlan{Required: due.copy, Bytes: due.bytes, FreeBytes: due.free, Kept: due.kept},
	}
	if last, ok := read.view.lastRun(); ok {
		report.Last = &last
	}
	return report, nil
}

// refusal is why this binary must not migrate a home.
type refusal struct {
	reason  HaltReason
	message string
}

// homeRead is the layout and ledger of a home as found on disk.
type homeRead struct {
	layout  int
	view    ledgerView
	refusal *refusal
}

// readHome reads LAYOUT and the ledger without changing anything. Begin and
// Inspect refuse by it, so they refuse the same homes for the same reasons.
func readHome(
	h home.Home,
	cat *Catalog,
) homeRead {
	layout, err := readLayout(h)
	if err != nil {
		return homeRead{refusal: &refusal{HaltLayoutUnreadable, err.Error()}}
	}
	if known := cat.Known(); layout > known {
		return homeRead{layout: layout, refusal: &refusal{HaltLayoutNewer, fmt.Sprintf(
			"this home is at layout %d but this crowbar only knows layout %d: run a newer crowbar", layout, known)}}
	}
	view, err := readLedger(ledgerPath(h))
	if err != nil {
		return homeRead{layout: layout, refusal: &refusal{HaltLedgerCorrupt, err.Error()}}
	}
	return homeRead{layout: layout, view: view}
}

func ledgerPath(
	h home.Home,
) string {
	return filepath.Join(h.MigrateDir(), ledgerName)
}

// haltAt is a Halt with everything the files alone tell: the layouts and the
// ledger's path where one exists. A session adds what only it knows.
func haltAt(
	h home.Home,
	cat *Catalog,
	layout int,
	reason HaltReason,
	message string,
) *Halt {
	halt := &Halt{Reason: reason, Message: message, Layout: layout, Known: cat.Known()}
	if _, err := os.Stat(ledgerPath(h)); err == nil {
		halt.Ledger = ledgerPath(h)
	}
	return halt
}

// at names the step the halt is about.
func (h *Halt) at(
	step Step,
) *Halt {
	id := step.ID()
	h.Step, h.Phase = &id, step.Phase.String()
	return h
}
