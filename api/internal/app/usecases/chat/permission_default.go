package chat

import "context"

// Crowbar's own guarded/trusted/full-auto names — plain strings, not a
// dedicated type, matching Model/Effort. What a specific PROVIDER means by
// each one is that provider's own descriptor's business (permission_levels
// in its own YAML); Go carries the name through unexamined.
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
