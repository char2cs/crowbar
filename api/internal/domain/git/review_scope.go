package git

// ReviewScope is a branch review's base ref, changed files and hunk Outline as ONE answer: base
// resolution costs up to three git subprocesses, and separate calls could straddle a push and
// disagree. The outline gives comments line anchors; it costs a streamed diff, hence outlineCache.
type ReviewScope struct {
	Base    string              `json:"base"`
	Files   []ReviewFileSummary `json:"files"`
	Outline []FileOutline       `json:"outline"`
}
