package git

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

type ConflictResolution string

const (
	ConflictResolutionOurs       ConflictResolution = "ours"
	ConflictResolutionTheirs     ConflictResolution = "theirs"
	ConflictResolutionBoth       ConflictResolution = "both"
	ConflictResolutionCustom     ConflictResolution = "custom"
	ConflictResolutionUnresolved ConflictResolution = "unresolved"
)
