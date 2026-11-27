package git

type SearchHit struct {
	Path       string `json:"path"`
	Side       string `json:"side"`
	LineNumber int    `json:"lineNumber"`
	Preview    string `json:"preview"`
}

const (
	SearchSideOld = "old"
	SearchSideNew = "new"
)

type SearchOpts struct {
	Regex         bool
	CaseSensitive bool
	Limit         int
}
