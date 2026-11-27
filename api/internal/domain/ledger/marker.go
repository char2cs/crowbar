package ledger

const (
	MarkerSessionChanged    = "session_changed"
	MarkerSessionConflict   = "session_conflict"
	MarkerDeadLetter        = "dead_letter"
	MarkerCompactionStarted = "compaction_started"
	MarkerCompactionEnded   = "compaction_ended"
	MarkerStopped           = "stopped"
	MarkerProviderSwitched  = "provider_switched"
	MarkerModelChanged      = "model_changed"
	MarkerEffortChanged     = "effort_changed"
	MarkerNotification      = "notification"
	MarkerWarning           = "warning"
	MarkerUnreadable        = "legacy_unreadable"
	MarkerFromRecord        = "from_record"
)

var markerKinds = []string{
	MarkerSessionChanged, MarkerSessionConflict, MarkerDeadLetter, MarkerCompactionStarted, MarkerCompactionEnded,
	MarkerStopped, MarkerProviderSwitched, MarkerModelChanged, MarkerEffortChanged, MarkerNotification, MarkerWarning,
	MarkerUnreadable, MarkerFromRecord,
}

type Marker struct {
	Kind           string   `json:"kind"`
	Detail         string   `json:"detail,omitempty"`
	Records        []string `json:"records,omitempty"`
	Owner          string   `json:"owner,omitempty"`
	ProviderTurnID string   `json:"providerTurnId,omitempty"`
}

type compaction struct {
	open           bool
	providerTurnID string
	turnID         string
}

func (s State) CompactionIn(turnID string) bool {
	return s.keys.compaction.open && turnID != "" && s.keys.compaction.turnID == turnID
}

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
		s.keys.compaction = compaction{open: true, providerTurnID: body.ProviderTurnID, turnID: r.TurnID}
	case MarkerCompactionEnded:
		s.keys.compaction.open = false
		if body.ProviderTurnID != "" && body.ProviderTurnID == s.keys.compaction.providerTurnID {
			s.keys.compaction.providerTurnID = ""
		}
	}
}
