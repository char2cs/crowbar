package ledger

// Marker kinds the transcript shows between turns.
const (
	MarkerSessionChanged    = "session_changed"  // the CLI moved the chat onto another session of its own
	MarkerSessionConflict   = "session_conflict" // the CLI entered a session another chat owns
	MarkerDeadLetter        = "dead_letter"      // the CLI reported something Crowbar could not map
	MarkerCompactionStarted = "compaction_started"
	MarkerCompactionEnded   = "compaction_ended"
	MarkerStopped           = "stopped"           // a person stopped the running turn
	MarkerProviderSwitched  = "provider_switched" // Detail: the provider switched to
	MarkerModelChanged      = "model_changed"     // Detail: the model selected
	MarkerEffortChanged     = "effort_changed"    // Detail: the effort selected
	MarkerNotification      = "notification"      // Detail: what the CLI told its user
)

// Marker is the body of a marker row: a fact the transcript shows between
// turns, named by Kind, with Detail as its subject.
type Marker struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail,omitempty"`
	// ProviderTurnID is the provider turn a compaction runs as its own: that
	// turn's end is the compaction's, never a reply.
	ProviderTurnID string `json:"providerTurnId,omitempty"`
}

// compaction is the chat's compaction in flight, and the provider turn whose
// end closes it.
type compaction struct {
	open           bool
	providerTurnID string
}

// Compaction reports whether a compaction is open, and the provider turn id
// whose end is the compaction's ("" when none is armed).
func (s State) Compaction() (open bool, providerTurnID string) {
	return s.keys.compaction.open, s.keys.compaction.providerTurnID
}

func (s *State) applyMarker(r Row) {
	body, err := DecodeBody[Marker](r)
	if err != nil {
		return
	}
	switch body.Kind {
	case MarkerCompactionStarted:
		s.keys.compaction = compaction{open: true, providerTurnID: body.ProviderTurnID}
	case MarkerCompactionEnded:
		s.keys.compaction.open = false
		if body.ProviderTurnID != "" && body.ProviderTurnID == s.keys.compaction.providerTurnID {
			s.keys.compaction.providerTurnID = ""
		}
	}
}
