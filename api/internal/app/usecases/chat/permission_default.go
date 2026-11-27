package chat

import "context"

const (
	PermissionGuarded  = "guarded"
	PermissionTrusted  = "trusted"
	PermissionFullAuto = "full-auto"
)

func (u *Usecase) DefaultPermissionLevel(
	ctx context.Context,
) (string, error) {
	return u.defaultLevel.Get(ctx)
}

func (u *Usecase) SetDefaultPermissionLevel(
	ctx context.Context,
	level string,
) error {
	return u.defaultLevel.Set(ctx, level)
}
