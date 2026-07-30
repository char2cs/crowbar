package git

// ReviewScope is what a branch review covers: the ref it diffs against and the
// files that differ from it, as ONE answer.
//
// The pair travels together because it is produced together. Resolving the base
// ref is not a lookup — it probes both origin/<base> and the local <base> and
// compares the two, which is up to three git subprocesses, and a subprocess
// costs about the same whatever it computes. Asking for the base and then
// asking for the files pays that resolution twice to answer a single question.
//
// It is also the only shape that can guarantee the two halves agree: two
// independent resolutions can straddle a push to the base branch and report a
// file list against a ref they were not diffed from.
type ReviewScope struct {
	Base  string              `json:"base"`
	Files []ReviewFileSummary `json:"files"`
}
