package ledger

import (
	"cmp"
	"slices"
)

const (
	PlanReplace = "replace"
	PlanUpsert  = "upsert"
)

type Plan struct {
	Op    string     `json:"op"`
	Items []PlanItem `json:"items"`
}

type PlanItem struct {
	ID     string `json:"id,omitempty"`
	Text   string `json:"text"`
	Detail string `json:"detail,omitempty"`
	Status string `json:"status,omitempty"`
}

const PlanStepRemoved = "removed"

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

func (s State) Plan() []PlanItem { return slices.Clone(s.plan) }
