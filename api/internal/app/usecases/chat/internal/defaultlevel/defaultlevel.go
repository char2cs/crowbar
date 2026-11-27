package defaultlevel

import (
	"context"
	"fmt"
	"slices"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func Valid(level string) bool {
	return slices.Contains(domain.PermissionLevels(), level)
}

type DefaultLevel struct {
	prefs store.Store[domain.AgentPermissionDefault, string]
}

type Deps struct {
	Prefs store.Store[domain.AgentPermissionDefault, string]
}

func New(
	d Deps,
) *DefaultLevel {
	return &DefaultLevel{prefs: d.Prefs}
}

func (d *DefaultLevel) Get(
	ctx context.Context,
) (string, error) {
	row, err := d.prefs.FindByKey(ctx, domain.DefaultPermissionLevelKey)
	if err != nil {
		return "", fmt.Errorf("agent: default permission level: %w", err)
	}
	if row == nil {
		return "full-auto", nil
	}
	return row.Level, nil
}

func (d *DefaultLevel) Set(
	ctx context.Context,
	level string,
) error {
	if !Valid(level) {
		return fmt.Errorf("%w: unknown permission level %q", problem.ErrInvalidArgument, level)
	}
	if err := d.prefs.Save(ctx, domain.AgentPermissionDefault{
		ID: domain.DefaultPermissionLevelKey, Level: level,
	}); err != nil {
		return fmt.Errorf("agent: set default permission level: %w", err)
	}
	return nil
}
