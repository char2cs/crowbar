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

type checked struct {
	plans  []Plan
	backup backupDue
}

type backupDue struct {
	step  Step
	copy  bool
	bytes int64
	free  uint64
	kept  string
	gone  string
	extra []string
}

func (s *Session) preflight(
	ctx context.Context,
	pending []Step,
) error {
	if err := ctx.Err(); err != nil {
		return s.interrupt(err)
	}
	res, err := checkPending(ctx, s.env(), pending, s.view(), s.opts)
	var f *fault
	switch {
	case errors.As(err, &f):
		return s.failUnstarted(f.reason, f.step, f.cause)
	case err != nil:
		return s.interrupt(err)
	}
	return s.backUpState(res.backup)
}

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
		env.Earlier, env.Notes = view.checkpoints(step.Number, step.Name), view.notes(step.Number, step.Name)
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
	for _, p := range plans {
		due.extra = append(due.extra, p.BacksUp...)
	}
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
	stats, err := measureBackup(h, due.extra)
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

func (s *Session) backUpState(
	due backupDue,
) error {
	if due.gone != "" {
		s.opts.Log.Warn("migrate: the backup this migration started from is gone; taking a new one", "backup", due.gone)
	}
	if !due.copy {
		return nil
	}
	if err := s.takeBackup(due.extra); err != nil {
		return s.failUnstarted(HaltPreflight, due.step, err)
	}
	return nil
}

func (s *Session) takeBackup(
	extra []string,
) error {
	res, err := backupState(s.home, s.run.id, s.layout, extra, s.opts.FreeBytes)
	if err != nil || res.Path == "" {
		return err
	}
	err = s.led.append(record{Kind: kindBackup, At: s.now(), Run: s.run.id, Path: res.Path, Bytes: res.Bytes})
	if err != nil {
		return errors.Join(fmt.Errorf("record backup: %w", err), removeTree(res.Path))
	}
	s.opts.Log.Info("migrate: backed up the home before migrating it", "backup", res.Path,
		"bytes", res.Bytes, "packed", res.Packed, "restore", restoreHint(s.home, res.Path))
	return writeRestoreNote(s.home)
}

func backupOnDisk(
	path string,
) bool {
	on, err := backupExists(path)
	return err == nil && on
}

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
