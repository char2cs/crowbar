package git

// ReviewFileSides is the full text of one changed file on each side of a review diff,
// so a reader can reveal the unchanged lines a patch's context leaves out. A nil side
// means the file does not exist there, which differs from an empty one.
type ReviewFileSides struct {
	Old *string `json:"old"`
	New *string `json:"new"`
}
