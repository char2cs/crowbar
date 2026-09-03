package models

type Selection struct {
	Model  string
	Effort string
	// PermissionLevel is Crowbar's own level name (guarded/trusted/full-auto),
	// never empty for a chat that has ever been seeded — unlike Model/Effort,
	// it has no "provider's own default" reading to fall back to.
	PermissionLevel string
}

func (s Selection) Empty() bool {
	return s.Model == "" && s.Effort == "" && s.PermissionLevel == ""
}
