package ledger

// Marker kinds the transcript shows between turns.
const (
	MarkerSessionChanged  = "session_changed"  // the CLI moved the chat onto another session of its own
	MarkerSessionConflict = "session_conflict" // the CLI entered a session another chat owns
	MarkerDeadLetter      = "dead_letter"      // the CLI reported something Crowbar could not map
)

// Marker is the body of a marker row: a fact the transcript shows between
// turns, named by Kind, with Detail as its subject.
type Marker struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail,omitempty"`
}
