package git

// ReviewScope is what a branch review covers: the ref it diffs against, the
// files that differ from it, and the hunk geometry of those differences, as ONE
// answer.
//
// The parts travel together because they are produced together. Resolving the
// base ref is not a lookup — it probes both origin/<base> and the local <base>
// and compares the two, which is up to three git subprocesses, and a subprocess
// costs about the same whatever it computes. Asking for the base and then
// asking for the files pays that resolution twice to answer a single question.
//
// It is also the only shape that can guarantee the parts agree: two independent
// resolutions can straddle a push to the base branch and report a file list
// against a ref they were not diffed from, or geometry describing a diff the
// file list never saw.
//
// Outline is that geometry — one entry per file of the SAME diff, carrying its
// `@@` shapes and no content. It is here rather than left to a second call
// because the scope is what a reader anchors against: without line numbers,
// "src/auth.go changed" is a file a comment can be attached to at a line the
// reader has to guess. Producing it costs a streamed read of the diff, which is
// why it is memoised (see outlineCache) rather than merely recomputed.
type ReviewScope struct {
	Base    string              `json:"base"`
	Files   []ReviewFileSummary `json:"files"`
	Outline []FileOutline       `json:"outline"`
}
