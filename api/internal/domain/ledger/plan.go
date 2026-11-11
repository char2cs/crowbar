package ledger

import (
	"cmp"
	"slices"
)

// How a plan row changes the plan.
const (
	PlanReplace = "replace" // Items is the whole plan
	PlanUpsert  = "upsert"  // Items are changed or added by id
)

// Plan is the body of a plan row: the agent's running to-do list.
type Plan struct {
	Op    string     `json:"op"`
	Items []PlanItem `json:"items"`
}

// PlanItem is one step of a plan.
type PlanItem struct {
	ID     string `json:"id,omitempty"`
	Text   string `json:"text"`
	Detail string `json:"detail,omitempty"`
	Status string `json:"status,omitempty"`
}

// PlanStepRemoved is the status of an upserted step that leaves the plan.
const PlanStepRemoved = "removed"

// FoldPlan is items after p: a replace is the whole plan; an upsert changes
// each step it names by id, keeping what it leaves out, and adds the rest.
func FoldPlan(items []PlanItem, p Plan) []PlanItem {
	if p.Op != PlanUpsert {
		return slices.Clone(p.Items)
	}
	out := slices.Clone(items)
	for _, step := range p.Items {
		at := slices.IndexFunc(out, func(i PlanItem) bool { return i.ID == step.ID && step.ID != "" })
		switch {
		case step.Status == PlanStepRemoved && at >= 0:
			out = slices.Delete(out, at, at+1)
		case step.Status == PlanStepRemoved:
		case at >= 0:
			cur := &out[at]
			cur.Text, cur.Detail, cur.Status = cmp.Or(step.Text, cur.Text), cmp.Or(step.Detail, cur.Detail), cmp.Or(step.Status, cur.Status)
		default:
			out = append(out, step)
		}
	}
	return out
}

func (s *State) applyPlan(r Row) {
	if body, err := DecodeBody[Plan](r); err == nil && r.Scope == "" {
		s.plan = FoldPlan(s.plan, body)
	}
}

// Plan is the main agent's plan as its plan rows left it: a task list it keeps
// across turns, unless a restatement replaces it whole.
func (s State) Plan() []PlanItem { return slices.Clone(s.plan) }
