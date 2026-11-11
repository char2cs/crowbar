package git

// ConflictHunk is a three-way conflict block within a conflicting file.
type ConflictHunk struct {
	ID              string             `json:"id"`
	StartLine       int                `json:"startLine"`
	EndLine         int                `json:"endLine"`
	Ours            string             `json:"ours"`
	Theirs          string             `json:"theirs"`
	Base            string             `json:"base,omitempty"`
	Resolution      ConflictResolution `json:"resolution"`
	ResolvedContent string             `json:"resolvedContent,omitempty"`
}

// ConflictResolution describes how a conflict hunk is resolved.
type ConflictResolution string

const (
	ConflictResolutionOurs       ConflictResolution = "ours"
	ConflictResolutionTheirs     ConflictResolution = "theirs"
	ConflictResolutionBoth       ConflictResolution = "both"
	ConflictResolutionCustom     ConflictResolution = "custom"
	ConflictResolutionUnresolved ConflictResolution = "unresolved"
)
