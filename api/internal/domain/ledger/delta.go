package ledger

type FeedDelta struct {
	ID      string `json:"id"`
	TurnID  string `json:"turnId"`
	Scope   string `json:"scope,omitempty"`
	Stream  string `json:"stream"`
	N       int    `json:"n"`
	Text    string `json:"text"`
	Replace bool   `json:"replace,omitempty"`
}
