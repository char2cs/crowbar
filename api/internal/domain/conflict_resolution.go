package domain

// ConflictResolution describes how a conflict hunk is resolved (04 §6).
type ConflictResolution string

const (
	ConflictResolutionOurs       ConflictResolution = "ours"
	ConflictResolutionTheirs     ConflictResolution = "theirs"
	ConflictResolutionBoth       ConflictResolution = "both"
	ConflictResolutionCustom     ConflictResolution = "custom"
	ConflictResolutionUnresolved ConflictResolution = "unresolved"
)
