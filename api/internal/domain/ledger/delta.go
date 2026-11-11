package ledger

// FeedDelta is one frame of a stream no row holds: only the text new since its
// stream's last frame, numbered per stream, or the whole text when Replace.
type FeedDelta struct {
	ID      string `json:"id"` // message id (answer, reasoning) or tool id (tool_output, diff)
	TurnID  string `json:"turnId"`
	Scope   string `json:"scope,omitempty"`
	Stream  string `json:"stream"` // "answer" | "reasoning" | "tool_output" | "diff"
	N       int    `json:"n"`      // 1-based position of this frame in its stream
	Text    string `json:"text"`   // the new suffix; everything so far when Replace
	Replace bool   `json:"replace,omitempty"`
}
