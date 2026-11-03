package git

// SearchHit is one match against a diff's CONTENT lines (never its headers). LineNumber counts in
// the file on Side (context lines on "new", where the reader navigates); Preview is capped so a
// minified line keeps the response bounded, and may stop before the match.
type SearchHit struct {
	Path       string `json:"path"`
	Side       string `json:"side"`
	LineNumber int    `json:"lineNumber"`
	Preview    string `json:"preview"`
}

// SearchSideOld and SearchSideNew are the two values SearchHit.Side takes.
const (
	SearchSideOld = "old"
	SearchSideNew = "new"
)

// SearchOpts tunes a diff search. An invalid Regex (RE2) is an error, never a panic: find-as-you-type
// submits half-typed patterns. Limit stops reading once filled, keeping broad queries bounded;
// Limit <= 0 is unlimited and only safe for narrow queries.
type SearchOpts struct {
	Regex         bool
	CaseSensitive bool
	Limit         int
}
