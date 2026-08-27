// Package defaultlevel owns the one global row holding the permission level a
// newly created chat is seeded with (see conversation.Deps.DefaultPermissionLevel,
// Task 6).
package defaultlevel

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// valid is the closed set Set accepts. Anything else — a typo, a future level
// not yet shipped — is rejected rather than silently stored, so Get can never
// hand a chat-seeding caller a Level permission.AutoApprove doesn't know how
// to interpret.
var valid = map[permission.Level]bool{
	permission.Guarded:  true,
	permission.Trusted:  true,
	permission.FullAuto: true,
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
// yet) reports permission.FullAuto — the shipped out-of-the-box default —
// not an error.
func (d *DefaultLevel) Get(
	ctx context.Context,
) (permission.Level, error) {
	row, err := d.prefs.FindByKey(ctx, domain.DefaultPermissionLevelKey)
	if err != nil {
		return "", fmt.Errorf("agent: default permission level: %w", err)
	}
	if row == nil {
		return permission.FullAuto, nil
	}
	return permission.Level(row.Level), nil
}

// Set overwrites the global default. It rejects any level outside the closed
// set before writing anything.
func (d *DefaultLevel) Set(
	ctx context.Context,
	level permission.Level,
) error {
	if !valid[level] {
		return fmt.Errorf("%w: unknown permission level %q", apperr.ErrInvalidArgument, level)
	}
	if err := d.prefs.Save(ctx, domain.AgentPermissionDefault{
		ID: domain.DefaultPermissionLevelKey, Level: string(level),
	}); err != nil {
		return fmt.Errorf("agent: set default permission level: %w", err)
	}
	return nil
}
