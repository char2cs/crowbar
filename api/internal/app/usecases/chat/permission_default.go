package chat

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
)

// PermissionLevel is the trust dial permission.Level names, re-exported here
// so callers outside this feature — the HTTP handlers — can name it without
// reaching past this feature's internal package boundary.
type PermissionLevel = permission.Level

const (
	PermissionGuarded  = permission.Guarded
	PermissionTrusted  = permission.Trusted
	PermissionFullAuto = permission.FullAuto
)

// DefaultPermissionLevel is the level a newly created chat is seeded with.
func (u *Usecase) DefaultPermissionLevel(
	ctx context.Context,
) (permission.Level, error) {
	return u.defaultLevel.Get(ctx)
}

// SetDefaultPermissionLevel overwrites the global default.
func (u *Usecase) SetDefaultPermissionLevel(
	ctx context.Context,
	level permission.Level,
) error {
	return u.defaultLevel.Set(ctx, level)
}
