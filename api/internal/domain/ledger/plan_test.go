package ledger_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func TestFold_PlanUpsertAndReplace(t *testing.T) {
	var items []ledger.PlanItem
	items = ledger.FoldPlan(items, ledger.Plan{Op: ledger.PlanUpsert, Items: []ledger.PlanItem{{ID: "1", Text: "read", Status: "pending"}}})
	items = ledger.FoldPlan(items, ledger.Plan{Op: ledger.PlanUpsert, Items: []ledger.PlanItem{{ID: "2", Text: "write", Status: "pending"}}})
	items = ledger.FoldPlan(items, ledger.Plan{Op: ledger.PlanUpsert, Items: []ledger.PlanItem{{ID: "1", Status: "active"}}})

	assert.Equal(t, []ledger.PlanItem{
		{ID: "1", Text: "read", Status: "active"}, {ID: "2", Text: "write", Status: "pending"},
	}, items, "an upsert changes what it names, keeping what it leaves out")

	items = ledger.FoldPlan(items, ledger.Plan{Op: ledger.PlanUpsert, Items: []ledger.PlanItem{{ID: "2", Status: ledger.PlanStepRemoved}}})
	assert.Equal(t, []ledger.PlanItem{{ID: "1", Text: "read", Status: "active"}}, items, "a removed step leaves the plan")

	items = ledger.FoldPlan(items, ledger.Plan{Op: ledger.PlanReplace, Items: []ledger.PlanItem{{Text: "all new"}}})
	assert.Equal(t, []ledger.PlanItem{{Text: "all new"}}, items, "a replace is the whole plan")
}

// The state keeps the main agent's plan, steps folded in, across its turns.
func TestState_KeepsTheMainAgentsPlan(t *testing.T) {
	step := func(turnID, id, status string) ledger.Row {
		return row(t, ledger.KindPlan, turnID, ledger.Plan{Op: ledger.PlanUpsert, Items: []ledger.PlanItem{{ID: id, Text: id, Status: status}}})
	}
	st := ledger.Fold("c1", sequenced(t, step("T1", "a", "pending"), step("T1", "b", "pending"), step("T1", "a", "active"),
		scoped(t, ledger.KindPlan, "T1", "A", ledger.Plan{Op: ledger.PlanReplace})))

	assert.Equal(t, []ledger.PlanItem{{ID: "a", Text: "a", Status: "active"}, {ID: "b", Text: "b", Status: "pending"}}, st.Plan(),
		"a subagent's plan is its own")

	next := step("T2", "c", "pending")
	next.Seq = st.Head() + 1
	st.Apply(next)
	assert.Len(t, st.Plan(), 3, "a task list outlives the turn that made it")
	again := row(t, ledger.KindPlan, "T2", ledger.Plan{Op: ledger.PlanReplace, Items: []ledger.PlanItem{{Text: "only"}}})
	again.Seq = st.Head() + 1
	st.Apply(again)
	assert.Equal(t, []ledger.PlanItem{{Text: "only"}}, st.Plan(), "a restatement replaces it whole")
}
