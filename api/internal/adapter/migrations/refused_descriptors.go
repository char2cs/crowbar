package migrations

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strconv"

	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

// StepRefusedDescriptors moves aside, once, each override under descriptors/ that the
// current schema refuses while a shipped descriptor runs in its place: kept as
// <name>.refused for the user, it no longer warns at every boot.
const StepRefusedDescriptors = 12

const refusedSuffix = ".refused"

// refusedOverrides is the path of every override refused in favour of a shipped default.
func refusedOverrides(home string) ([]string, error) {
	reports, err := descriptorcheck.ValidateAll(home)
	if err != nil {
		return nil, fmt.Errorf("refused descriptors: %w", err)
	}
	var out []string
	for _, r := range reports {
		if r.FellBack {
			out = append(out, r.Source)
		}
	}
	return out, nil
}

func planRefusedDescriptors(_ context.Context, env migrate.Env) (migrate.Plan, error) {
	paths, err := refusedOverrides(env.Home.Root())
	if err != nil || len(paths) == 0 {
		return migrate.Plan{}, err
	}
	return migrate.Plan{Actions: []migrate.Action{{
		Kind: migrate.Convert, Subject: "descriptor overrides this build refuses → <name>.refused", Count: int64(len(paths)),
	}}}, nil
}

func retireRefusedDescriptors(ctx context.Context, env migrate.Env, _ *migrate.Checkpoints) (migrate.Counts, error) {
	paths, err := refusedOverrides(env.Home.Root())
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		target, err := freeName(path + refusedSuffix)
		if err != nil {
			return nil, err
		}
		if err := os.Rename(path, target); err != nil {
			return nil, fmt.Errorf("refused descriptors: %w", err)
		}
		slog.WarnContext(ctx, "migrate: descriptor override refused by this build; moved aside, the shipped descriptor runs",
			"from", path, "to", target)
	}
	return migrate.Counts{"retired": int64(len(paths))}, nil
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
			return "", fmt.Errorf("refused descriptors: %w", err)
		}
		candidate = name + "." + strconv.Itoa(n)
	}
}
