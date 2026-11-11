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
	// Backups are the backups on disk, oldest first, and RestoreNote the file
	// beside them that says how to restore one.
	Backups     []BackupFile
	RestoreNote string
}

// BackupFile is one backup on disk and the bytes it takes.
type BackupFile struct {
	Path  string
	Bytes int64
}

// Inspect previews, for a build that implements the steps implemented reports,
// the migration of h against cat without changing h (no lock beyond a probe, no
// repair). A home or step the boot would halt on comes back as the boot's *Halt.
func Inspect(
	ctx context.Context,
	h home.Home,
	cat *Catalog,
	implemented func(number int) bool,
	opts Options,
) (Report, error) {
	opts = opts.withDefaults()
	read := readHome(h, cat)
	if read.refusal != nil {
		return Report{}, haltAt(h, cat, read.layout, read.view, read.refusal.reason, read.refusal.message)
	}
	view := cat.settled(read.layout, read.view)
	halt := func(f *fault) *Halt {
		return haltAt(h, cat, read.layout, view, f.reason, haltMessage(f.reason, f.step, opts.plain(f.cause))).at(f.step)
	}
	env := Env{Home: h, Log: opts.Log, History: History{home: h, view: view}}
	pending := cat.pending(read.layout, view, cat.recurring(ctx, env, read.layout, view))
	// In the boot's order: RunPreOpen admits its steps before the preflight, and
	// RunPostOpen admits the rest after it.
	if step, ok := unimplemented(leading(pending, PreOpen), implemented); ok {
		return Report{}, halt(&fault{HaltNoImpl, step, errNoImpl})
	}
	res, err := checkPending(ctx, env, pending, view, opts)
	var f *fault
	if errors.As(err, &f) {
		return Report{}, halt(f)
	}
	if err != nil {
		return Report{}, err
	}
	if step, ok := unimplemented(pending, implemented); ok {
		return Report{}, halt(&fault{HaltNoImpl, step, errNoImpl})
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
	backups, err := listBackups(h)
	if err != nil {
		return Report{}, err
	}
	for _, path := range backups {
		report.Backups = append(report.Backups, BackupFile{Path: path, Bytes: treeSize(path)})
	}
	report.RestoreNote = filepath.Join(h.MigrateDir(), backupsDirName, restoreNoteName)
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
			"this home is at layout %d but this Crowbar only knows layout %d: run a newer Crowbar", layout, known)}}
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

// haltAt is a Halt with everything the files alone tell: the layouts, the ledger
// where one exists and the backup in force, which the message names too, since
// the message is all a person may see. A session adds its run.
func haltAt(
	h home.Home,
	cat *Catalog,
	layout int,
	view ledgerView,
	reason HaltReason,
	message string,
) *Halt {
	halt := &Halt{Reason: reason, Message: message, Layout: layout, Known: cat.Known()}
	if _, err := os.Stat(ledgerPath(h)); err == nil {
		halt.Ledger = ledgerPath(h)
	}
	if path := view.backupInForce().Path; backupOnDisk(path) {
		halt.Backup = path
		halt.Message = fmt.Sprintf("%s (your data from before this migration is backed up in %s)", message, path)
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
