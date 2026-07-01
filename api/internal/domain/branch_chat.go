package domain

// BranchChat is a lightweight projection of a Chat (01) surfaced read-only
// inside the branch review panel (09 §2). Title and Age are derived fields.
type BranchChat struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Age   string `json:"age"`
}
