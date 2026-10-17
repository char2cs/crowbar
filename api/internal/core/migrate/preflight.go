package migrate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
)

// preflight is everything a run checks and secures before its first step, so a
// boot that is going to fail fails before it changes anything: every pending
// step of both phases is planned offline, what the plans need is checked, and
// state/ is backed up when a plan will rewrite it.
func (s *Session) preflight(
	ctx context.Context,
	pending []Step,
) error {
	if err := ctx.Err(); err != nil {
		return s.interrupt(err)
	}
	plans, err := s.planAll(ctx, pending)
	if err != nil {
		return err
	}
	if err := s.checkGit(pending, plans); err != nil {
		return err
	}
	return s.backUpState(pending, plans)
}

func (s *Session) planAll(
	ctx context.Context,
	pending []Step,
) ([]Plan, error) {
	env := Env{Home: s.home, Log: s.opts.Log}
	plans := make([]Plan, 0, len(pending))
	for _, step := range pending {
		plan, err := step.Plan(ctx, env)
		if interrupted(ctx, err) {
			return nil, s.interrupt(err)
		}
		if err != nil {
			return nil, s.failBeforeSteps(HaltPlanFailed, step, err)
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func (s *Session) checkGit(
	pending []Step,
	plans []Plan,
) error {
	i := slices.IndexFunc(plans, func(p Plan) bool { return p.NeedsGit })
	if i < 0 || s.opts.HaveGit() {
		return nil
	}
	return s.failBeforeSteps(HaltPreflight, pending[i], errors.New("git is required but was not found on PATH"))
}

// backUpState backs up state/ when a plan will rewrite it.
func (s *Session) backUpState(
	pending []Step,
	plans []Plan,
) error {
	i := slices.IndexFunc(plans, func(p Plan) bool { return p.RewritesState })
	if i < 0 {
		return nil
	}
	if err := s.takeBackup(); err != nil {
		return s.failBeforeSteps(HaltPreflight, pending[i], err)
	}
	return nil
}

// takeBackup copies state/ once per migration attempt: while the ledger holds a
// backup no completed run has followed, a retry keeps it, so partially migrated
// state never replaces the image a person would restore from.
func (s *Session) takeBackup() error {
	kept, err := backupExists(s.view().backupInForce())
	if err != nil || kept {
		return err
	}
	res, err := backupState(s.home, s.run.id, s.opts.FreeBytes)
	if err != nil || res.Path == "" {
		return err
	}
	err = s.led.append(record{Kind: kindBackup, At: s.now(), Run: s.run.id, Path: res.Path, Bytes: res.Bytes})
	if err != nil {
		return fmt.Errorf("record backup %s: %w", res.Path, err)
	}
	return nil
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
