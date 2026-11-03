// Package defaultlevel owns the one global row holding the permission level a
// newly created chat is seeded with (see conversation.Deps.DefaultPermissionLevel).
package defaultlevel

import (
	"context"
	"fmt"
	"slices"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// Valid keeps the global default, which has no provider to validate against
// yet, inside Crowbar's own level names.
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

// Get is the level a newly created chat is seeded with. Unset (no row saved
// yet) reports "full-auto" — the shipped out-of-the-box default — not an
// error.
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

// Set overwrites the global default. It rejects any level outside the closed
// set before writing anything.
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
