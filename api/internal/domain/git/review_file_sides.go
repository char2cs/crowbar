package git

// ReviewFileSides is the full text of one changed file on each side of a review
// diff. It is what lets a reader reveal the unchanged lines a patch leaves out:
// the patch carries three lines of context per hunk, everything else has to come
// from the file itself.
//
// A nil side means the file does not exist there (added or deleted), which is
// different from an empty one.
type ReviewFileSides struct {
	Old *string `json:"old"`
	New *string `json:"new"`
}
