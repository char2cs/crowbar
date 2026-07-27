package git

// SearchHit is one match of a query against the CONTENT of a branch diff — an
// added, removed or context line inside a hunk. The patch's own scaffolding
// (`diff --git`, `index`, `---`/`+++`, `@@` headers, "Binary files … differ")
// is never a hit: those lines are how the diff is spelled, not what it says.
//
// LineNumber is a line number in the file, not an offset into the patch, and
// Side says which side of the diff it counts on. A removed line is numbered on
// the "old" side; an added line on the "new" side; a context line exists on
// both and is reported on the "new" side, since that is the file the reader
// navigates to.
//
// Preview is the line's content with its diff prefix stripped, capped at a
// length that keeps a response bounded even when the match lands in a minified
// bundle whose single line runs to hundreds of kilobytes. A capped preview may
// therefore stop before the match itself.
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

// SearchOpts tunes a diff search.
//
// Regex reads the query as an RE2 pattern instead of literal text; an invalid
// pattern is an error, never a panic, because a find-as-you-type box submits
// every half-finished pattern the user passes through. CaseSensitive defaults
// to false, so the zero value is the forgiving search a find box wants.
//
// Limit caps how many hits are collected; the search stops reading the diff the
// moment it fills, which is what keeps a query like "e" over a million-line
// diff from being an out-of-memory. Limit <= 0 means unlimited, and is only
// safe for a caller that knows the query is narrow.
type SearchOpts struct {
	Regex         bool
	CaseSensitive bool
	Limit         int
}
