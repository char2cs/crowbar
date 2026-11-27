package git

type HunkShape struct {
	OldStart int `json:"oldStart"`
	OldLines int `json:"oldLines"`
	NewStart int `json:"newStart"`
	NewLines int `json:"newLines"`
}

type FileOutline struct {
	Path      string      `json:"path"`
	OldPath   string      `json:"oldPath,omitempty"`
	Hunks     []HunkShape `json:"hunks"`
	IsPartial bool        `json:"isPartial"`
	IsBinary  bool        `json:"isBinary"`
}
