package migrate

// ActionKind is what a planned action does to its subject.
type ActionKind string

const (
	// Convert rewrites the subject in place or into another shape.
	Convert ActionKind = "convert"
	// Delete removes the subject.
	Delete ActionKind = "delete"
)

// Unknown is a count the offline plan cannot know.
const Unknown int64 = -1

// Action is one line of a plan: what happens to Subject, and how many items.
type Action struct {
	Kind    ActionKind
	Subject string
	Dest    string
	Count   int64
	Note    string
}

// Plan is what a step would do, computed without writing anything.
type Plan struct {
	Actions []Action
	// RewritesState asks for a backup of state/ before the run.
	RewritesState bool
	NeedsGit      bool
}

// Empty reports that the plan has no actions.
func (p Plan) Empty() bool {
	return len(p.Actions) == 0
}
