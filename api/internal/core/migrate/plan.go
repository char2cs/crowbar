package migrate

type ActionKind string

const (
	Convert ActionKind = "convert"
	Delete  ActionKind = "delete"
	Record  ActionKind = "record"
)

const Unknown int64 = -1

type Action struct {
	Kind    ActionKind
	Subject string
	Count   int64
}

type Plan struct {
	Actions       []Action
	RewritesState bool
	BacksUp       []string
	NeedsGit      bool
}

func (p Plan) Empty() bool {
	return len(p.Actions) == 0
}
