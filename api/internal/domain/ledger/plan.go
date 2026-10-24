package ledger

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
