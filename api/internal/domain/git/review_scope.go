package git

type ReviewScope struct {
	Base    string              `json:"base"`
	Files   []ReviewFileSummary `json:"files"`
	Outline []FileOutline       `json:"outline"`
}
