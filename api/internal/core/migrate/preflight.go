package migrate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

// fault is a check the pending steps fail before the run writes anything, and
// the step to blame. It is the cause of the boot's halt, and of the same halt in
// a preview.
type fault struct {
	reason HaltReason
	step   Step
	cause  error
}

func (f *fault) Error() string {
	return haltMessage(f.reason, f.step, f.cause)
}

func (f *fault) Unwrap() error {
	return f.cause
}

// checked is what the preflight decided about the pending steps.
type checked struct {
	plans  []Plan
	backup backupDue
}

// backupDue is what the run does about state/ first: copies it (copy), keeps a
// backup an earlier attempt took (kept, that run's id), or neither. gone is a
// backup the ledger names that is no longer on disk.
type backupDue struct {
	step  Step
	copy  bool
	bytes int64
	free  uint64
	kept  string
	gone  string
}

// preflight plans every pending step of both phases offline, checks what the
// plans need and backs up state/ when one rewrites it, so a boot that will fail
// fails before it changes anything.
func (s *Session) preflight(
	ctx context.Context,
	pending []Step,
) error {
	if err := ctx.Err(); err != nil {
		return s.interrupt(err)
	}
	res, err := checkPending(ctx, Env{Home: s.home, Log: s.opts.Log}, pending, s.view(), s.opts)
	var f *fault
	switch {
	case errors.As(err, &f):
		return s.failUnstarted(f.reason, f.step, f.cause)
	case err != nil:
		return s.interrupt(err)
	}
	return s.backUpState(res.backup)
}

// checkPending plans the pending steps and makes every decision the run makes
// before it writes anything; the session acts on it and Inspect reports it. A
// failed check is a *fault; any other error is ctx giving up.
func checkPending(
	ctx context.Context,
	env Env,
	pending []Step,
	view ledgerView,
	opts Options,
) (checked, error) {
	plans, err := planSteps(ctx, env, pending, view)
	if err != nil {
		return checked{}, err
	}
	if i := slices.IndexFunc(plans, func(p Plan) bool { return p.NeedsGit }); i >= 0 && !opts.HaveGit() {
		return checked{}, &fault{HaltPreflight, pending[i], errors.New("git is required but was not found on PATH")}
	}
	backup, err := dueBackup(env.Home, view, opts.FreeBytes, pending, plans)
	if err != nil {
		return checked{}, err
	}
	return checked{plans: plans, backup: backup}, nil
}

func planSteps(
	ctx context.Context,
	env Env,
	pending []Step,
	view ledgerView,
) ([]Plan, error) {
	plans := make([]Plan, 0, len(pending))
	for _, step := range pending {
		env.Earlier = view.checkpoints(step.Number, step.Name)
		plan, err := step.Plan(ctx, env)
		if interrupted(ctx, err) {
			return nil, err
		}
		if err != nil {
			return nil, &fault{HaltPlanFailed, step, err}
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

// dueBackup is the one rule for the run's backup of state/: due when a plan
// rewrites it and it holds files, unless an earlier attempt's backup is still on
// disk. It fails as the run would, on an entry a backup cannot take or no room.
func dueBackup(
	h home.Home,
	view ledgerView,
	free FreeBytesFunc,
	pending []Step,
	plans []Plan,
) (backupDue, error) {
	i := slices.IndexFunc(plans, func(p Plan) bool { return p.RewritesState })
	if i < 0 {
		return backupDue{}, nil
	}
	due := backupDue{step: pending[i]}
	earlier := view.backupInForce()
	kept, err := backupExists(earlier.Path)
	if err != nil {
		return backupDue{}, &fault{HaltPreflight, due.step, err}
	}
	if kept {
		due.kept = earlier.Run
		return due, nil
	}
	due.gone = earlier.Path
	stats, err := measureState(h.State())
	if err != nil {
		return backupDue{}, &fault{HaltPreflight, due.step, err}
	}
	if stats.files == 0 {
		return due, nil
	}
	available, err := probeRoom(h.Root(), stats.bytes, free)
	if err != nil {
		return backupDue{}, &fault{HaltPreflight, due.step, err}
	}
	due.copy, due.bytes, due.free = true, stats.bytes, available
	return due, nil
}

// backUpState acts on dueBackup: one copy of state/ per migration attempt, kept
// across retries, so partly migrated state never replaces the image a person
// would restore from.
func (s *Session) backUpState(
	due backupDue,
) error {
	if due.gone != "" {
		s.opts.Log.Warn("migrate: the backup this migration started from is gone; taking a new one", "backup", due.gone)
	}
	if !due.copy {
		return nil
	}
	if err := s.takeBackup(); err != nil {
		return s.failUnstarted(HaltPreflight, due.step, err)
	}
	return nil
}

// takeBackup copies state/ and records the copy. A copy the ledger cannot
// record is removed: no halt would offer it, and no retry would reuse it.
func (s *Session) takeBackup() error {
	res, err := backupState(s.home, s.run.id, s.layout, s.opts.FreeBytes)
	if err != nil || res.Path == "" {
		return err
	}
	err = s.led.append(record{Kind: kindBackup, At: s.now(), Run: s.run.id, Path: res.Path, Bytes: res.Bytes})
	if err != nil {
		return errors.Join(fmt.Errorf("record backup: %w", err), removeTree(res.Path))
	}
	return nil
}

// backupOnDisk reports whether the backup at path is there to restore from.
func backupOnDisk(
	path string,
) bool {
	on, err := backupExists(path)
	return err == nil && on
}

// backupExists reports whether the backup at path is still on disk; no path
// means there is none.
func backupExists(
	path string,
) (bool, error) {
	if path == "" {
		return false, nil
	}
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("migrate: backup: inspect %s: %w", path, err)
	}
	return true, nil
}
