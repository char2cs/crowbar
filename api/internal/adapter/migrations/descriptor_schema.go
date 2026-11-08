package migrations

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

// StepDescriptorSchema rewrites each descriptor override under descriptors/ that
// was written for an older descriptor schema, keeping the original beside it,
// and first puts back an override an earlier build of this branch set aside as
// <name>.refused. It recurs, so an old file dropped in later is upgraded too.
const StepDescriptorSchema = 14

const (
	overrideDir   = "descriptors"
	overrideExt   = ".yaml"
	refusedSuffix = ".refused"
)

// descriptorWork is what the step would do: set-aside overrides to put back,
// and overrides whose schema is older than this build's.
type descriptorWork struct {
	restore []string
	upgrade []descriptorcheck.Source
}

func scanDescriptors(home string) (descriptorWork, error) {
	var w descriptorWork
	entries, err := os.ReadDir(filepath.Join(home, overrideDir))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return w, fmt.Errorf("descriptor schema: %w", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, overrideExt+refusedSuffix) {
			continue
		}
		_, err := os.Lstat(filepath.Join(home, overrideDir, strings.TrimSuffix(name, refusedSuffix)))
		if errors.Is(err, fs.ErrNotExist) {
			w.restore = append(w.restore, filepath.Join(home, overrideDir, name))
		}
	}
	sources, err := descriptorcheck.Sources(home)
	if err != nil {
		return w, fmt.Errorf("descriptor schema: %w", err)
	}
	for _, src := range sources {
		if src.Path == "" {
			continue
		}
		// A newer stamp is left as is; validation names it.
		if up, err := descriptorcheck.Upgrade(src.Raw); err == nil && len(up.Changes) > 0 {
			w.upgrade = append(w.upgrade, src)
		}
	}
	return w, nil
}

func planDescriptorSchema(_ context.Context, env migrate.Env) (migrate.Plan, error) {
	w, err := scanDescriptors(env.Home.Root())
	if err != nil {
		return migrate.Plan{}, err
	}
	var plan migrate.Plan
	if n := len(w.restore); n > 0 {
		plan.Actions = append(plan.Actions, migrate.Action{
			Kind: migrate.Convert, Subject: "descriptor overrides set aside as <name>.refused → put back", Count: int64(n),
		})
	}
	if len(w.upgrade) > 0 || len(w.restore) > 0 {
		count := int64(len(w.upgrade))
		if len(w.restore) > 0 {
			count = migrate.Unknown // a file put back may need upgrading too
		}
		plan.Actions = append(plan.Actions, migrate.Action{
			Kind:    migrate.Convert,
			Subject: "descriptor overrides written for an older schema → current schema, original kept as <name>.schema<N>.bak",
			Count:   count,
		})
	}
	return plan, nil
}

func upgradeDescriptors(ctx context.Context, env migrate.Env, _ *migrate.Checkpoints) (migrate.Counts, error) {
	home := env.Home.Root()
	w, err := scanDescriptors(home)
	if err != nil {
		return nil, err
	}
	for _, path := range w.restore {
		target := strings.TrimSuffix(path, refusedSuffix)
		if err := os.Rename(path, target); err != nil {
			return nil, fmt.Errorf("descriptor schema: put back %s: %w", path, err)
		}
		env.Log.WarnContext(ctx, "migrate: descriptor override put back from where an earlier build set it aside",
			"from", path, "to", target)
	}
	counts := migrate.Counts{"restored": int64(len(w.restore)), "rewritten": 0, "changes": 0}
	if len(w.restore) > 0 {
		if w, err = scanDescriptors(home); err != nil {
			return nil, err
		}
	}
	for _, src := range w.upgrade {
		n, err := upgradeOverride(ctx, env, src)
		if err != nil {
			return nil, err
		}
		counts["rewritten"]++
		counts["changes"] += int64(n)
	}
	return counts, nil
}

// upgradeOverride keeps src's bytes as <file>.schema<N>.bak, then replaces the
// file with its upgraded document, logging each change.
func upgradeOverride(ctx context.Context, env migrate.Env, src descriptorcheck.Source) (int, error) {
	up, err := descriptorcheck.Upgrade(src.Raw)
	if err != nil {
		return 0, fmt.Errorf("descriptor schema: %s: %w", src.Path, err)
	}
	backup, err := freeName(src.Path + ".schema" + strconv.Itoa(up.From) + ".bak")
	if err != nil {
		return 0, err
	}
	if err := writeNew(backup, src.Raw); err != nil {
		return 0, err
	}
	if err := replaceFile(src.Path, up.Raw); err != nil {
		return 0, err
	}
	for _, c := range up.Changes {
		env.Log.WarnContext(ctx, "migrate: descriptor override brought up to the current schema",
			"file", src.Path, "backup", backup, "key", c.Path, "line", c.Line, "change", c.Note)
	}
	return len(up.Changes), nil
}

func writeNew(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304: a name freeName chose under the daemon-owned home
	if err != nil {
		return fmt.Errorf("descriptor schema: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		return errors.Join(fmt.Errorf("descriptor schema: %w", err), f.Close())
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("descriptor schema: %w", err)
	}
	return nil
}

// replaceFile swaps data in for path atomically, so a crash leaves the old
// document or the new one, never half of either.
func replaceFile(path string, data []byte) error {
	tmp, err := freeName(path + ".tmp")
	if err != nil {
		return err
	}
	if err := writeNew(tmp, data); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return errors.Join(fmt.Errorf("descriptor schema: %w", err), os.Remove(tmp))
	}
	return nil
}

// freeName is name, or name.N for the first N from 2 that is free: a step run
// again on a restored home must not replace the copy the first run kept.
func freeName(name string) (string, error) {
	candidate := name
	for n := 2; ; n++ {
		_, err := os.Lstat(candidate)
		if errors.Is(err, fs.ErrNotExist) {
			return candidate, nil
		}
		if err != nil {
			return "", fmt.Errorf("descriptor schema: %w", err)
		}
		candidate = name + "." + strconv.Itoa(n)
	}
}
