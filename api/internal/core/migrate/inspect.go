package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

// StepPlan is a pending step with what it would do.
type StepPlan struct {
	Step Step
	Plan Plan
}

// BackupPlan says whether the run would copy state/ first, how much that copy
// weighs and how much room the volume has.
type BackupPlan struct {
	Required  bool
	Bytes     int64
	FreeBytes uint64
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

// Inspect plans the migration of h against cat. It is read-only: it takes no
// lock beyond a momentary probe, repairs no ledger, closes no run and creates
// nothing, so it also works on a home with no state/ or migrate/. A home a
// session would refuse comes back as the *Halt Begin would set.
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
	plans, err := planPending(ctx, h, cat, read, opts)
	if err != nil {
		return Report{}, err
	}
	backup, err := planBackup(h, plans, opts.FreeBytes)
	if err != nil {
		return Report{}, err
	}
	report := Report{
		Home: h.Root(), Layout: read.layout, Known: cat.Known(),
		LockHeld: probeLock(h), Steps: plans, Backup: backup,
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
// Inspect both decide from it, so they refuse the same homes for the same
// reasons: a LAYOUT unreadable or newer than cat knows, or a ledger corrupt or
// unreadable.
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

func planPending(
	ctx context.Context,
	h home.Home,
	cat *Catalog,
	read homeRead,
	opts Options,
) ([]StepPlan, error) {
	env := Env{Home: h, Log: opts.Log}
	pending := cat.pending(read.layout, read.view)
	plans := make([]StepPlan, 0, len(pending))
	for _, step := range pending {
		plan, err := step.Plan(ctx, env)
		if interrupted(ctx, err) {
			return nil, err
		}
		if err != nil {
			return nil, haltAt(h, cat, read.layout, HaltPlanFailed, haltMessage(HaltPlanFailed, step, err)).at(step)
		}
		plans = append(plans, StepPlan{Step: step, Plan: plan})
	}
	return plans, nil
}

// planBackup says whether a run would back up state/. backupState copies
// nothing out of an empty state/, so there is no backup to plan for one.
func planBackup(
	h home.Home,
	plans []StepPlan,
	free FreeBytesFunc,
) (BackupPlan, error) {
	if !slices.ContainsFunc(plans, func(p StepPlan) bool { return p.Plan.RewritesState }) {
		return BackupPlan{}, nil
	}
	size, err := treeSize(h.State())
	if err != nil {
		return BackupPlan{}, fmt.Errorf("migrate: inspect: measure %s: %w", h.State(), err)
	}
	if size == 0 {
		return BackupPlan{}, nil
	}
	available, err := free(h.Root())
	if err != nil {
		return BackupPlan{}, fmt.Errorf("migrate: inspect: probe free space under %s: %w", h.Root(), err)
	}
	return BackupPlan{Required: true, Bytes: size, FreeBytes: available}, nil
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
