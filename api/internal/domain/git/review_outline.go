package git

// HunkShape is one `@@` header reduced to start and count per side. Counts include context, so they give
// the rendered height the client reserves before fetching patches. An omitted count means one; zero
// means the side is absent.
type HunkShape struct {
	OldStart int `json:"oldStart"`
	OldLines int `json:"oldLines"`
	NewStart int `json:"newStart"`
	NewLines int `json:"newLines"`
}

// FileOutline is one file of a branch diff as hunk geometry only, O(hunks), so the client lays out every
// file first. Renames carry the new path in Path and the source in OldPath; binaries have no hunks.
// IsPartial means Hunks hit the cap: a lower bound, so the last hunk is not the file's end.
type FileOutline struct {
	Path      string      `json:"path"`
	OldPath   string      `json:"oldPath,omitempty"`
	Hunks     []HunkShape `json:"hunks"`
	IsPartial bool        `json:"isPartial"`
	IsBinary  bool        `json:"isBinary"`
}
