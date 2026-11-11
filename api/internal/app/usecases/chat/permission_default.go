package chat

import "context"

// Crowbar's guarded/trusted/full-auto names, plain strings like Model/Effort. What a
// provider means by each is its descriptor's permission_levels; Go carries them unexamined.
const (
	PermissionGuarded  = "guarded"
	PermissionTrusted  = "trusted"
	PermissionFullAuto = "full-auto"
)

// DefaultPermissionLevel is the level a newly created chat is seeded with.
func (u *Usecase) DefaultPermissionLevel(
	ctx context.Context,
) (string, error) {
	return u.defaultLevel.Get(ctx)
}

// SetDefaultPermissionLevel overwrites the global default.
func (u *Usecase) SetDefaultPermissionLevel(
	ctx context.Context,
	level string,
) error {
	return u.defaultLevel.Set(ctx, level)
}
