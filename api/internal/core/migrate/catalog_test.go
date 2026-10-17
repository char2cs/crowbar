package migrate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stubPlan(
	context.Context,
	Env,
) (Plan, error) {
	return Plan{}, nil
}

func newStep(
	number int,
	name string,
	phase Phase,
	from int,
	to int,
) Step {
	return Step{Number: number, Name: name, Phase: phase, From: from, To: to, Plan: stubPlan}
}

func numbers(
	steps []Step,
) []int {
	out := make([]int, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.Number)
	}
	return out
}

func neverApplied(
	Step,
) bool {
	return false
}

func chain() []Step {
	return []Step{
		newStep(1, "a", PreOpen, 0, 0),
		newStep(2, "b", PreOpen, 0, 1),
		newStep(3, "c", PreOpen, 1, 1),
		newStep(4, "d", PostOpen, 1, 1),
	}
}

func TestNewCatalog_AcceptsAChain(t *testing.T) {
	c, err := NewCatalog(chain()...)

	require.NoError(t, err)
	assert.Equal(t, 1, c.Known())
}

func TestNewCatalog_EmptyIsValid(t *testing.T) {
	c, err := NewCatalog()

	require.NoError(t, err)
	assert.Equal(t, 0, c.Known())
	assert.Empty(t, c.order(0, neverApplied))
}

func TestNewCatalog_KnownIsTheEndOfTheBumpChain(t *testing.T) {
	c, err := NewCatalog(
		newStep(1, "a", PreOpen, 0, 1),
		newStep(2, "b", PreOpen, 1, 2),
		newStep(3, "c", PreOpen, 2, 3),
	)

	require.NoError(t, err)
	assert.Equal(t, 3, c.Known())
}

func TestNewCatalog_Rejects(t *testing.T) {
	cases := []struct {
		name     string
		steps    []Step
		fragment string
	}{
		{
			name:     "duplicate number",
			steps:    []Step{newStep(1, "a", PreOpen, 0, 0), newStep(1, "b", PreOpen, 0, 0)},
			fragment: "duplicate step number",
		},
		{
			name:     "duplicate name",
			steps:    []Step{newStep(1, "a", PreOpen, 0, 0), newStep(2, "a", PreOpen, 0, 0)},
			fragment: "duplicate step name",
		},
		{
			name:     "name with capital and underscore",
			steps:    []Step{newStep(1, "Bad_Name", PreOpen, 0, 0)},
			fragment: "invalid step name",
		},
		{
			name:     "empty name",
			steps:    []Step{newStep(1, "", PreOpen, 0, 0)},
			fragment: "invalid step name",
		},
		{
			name:     "name starting with a digit",
			steps:    []Step{newStep(1, "1st", PreOpen, 0, 0)},
			fragment: "invalid step name",
		},
		{
			name:     "number zero",
			steps:    []Step{newStep(0, "a", PreOpen, 0, 0)},
			fragment: "step number must be >= 1",
		},
		{
			name:     "negative number",
			steps:    []Step{newStep(-3, "a", PreOpen, 0, 0)},
			fragment: "step number must be >= 1",
		},
		{
			name:     "to is from plus two",
			steps:    []Step{newStep(1, "a", PreOpen, 0, 2)},
			fragment: "invalid layout range",
		},
		{
			name:     "to is below from",
			steps:    []Step{newStep(1, "a", PreOpen, 2, 1)},
			fragment: "invalid layout range",
		},
		{
			name:     "negative from",
			steps:    []Step{newStep(1, "a", PreOpen, -1, 0)},
			fragment: "invalid layout range",
		},
		{
			name:     "unset phase",
			steps:    []Step{newStep(1, "a", Phase(0), 0, 0)},
			fragment: "invalid phase",
		},
		{
			name:     "post-open bump",
			steps:    []Step{newStep(1, "a", PostOpen, 0, 1)},
			fragment: "only pre-open steps may bump the layout",
		},
		{
			name:     "two bumps from one layout",
			steps:    []Step{newStep(1, "a", PreOpen, 0, 1), newStep(2, "b", PreOpen, 0, 1)},
			fragment: "more than one bump from layout",
		},
		{
			name:     "gap in the chain",
			steps:    []Step{newStep(1, "a", PreOpen, 0, 1), newStep(2, "b", PreOpen, 2, 3)},
			fragment: "gap in layout chain",
		},
		{
			name:     "chain not starting at the legacy layout",
			steps:    []Step{newStep(1, "a", PreOpen, 1, 2)},
			fragment: "gap in layout chain",
		},
		{
			name:     "post-open step below the known layout",
			steps:    []Step{newStep(1, "a", PreOpen, 0, 1), newStep(2, "b", PostOpen, 0, 0)},
			fragment: "post-open step must sit at the known layout",
		},
		{
			name:     "post-open step above the known layout",
			steps:    []Step{newStep(1, "a", PreOpen, 0, 1), newStep(2, "b", PostOpen, 2, 2)},
			fragment: "post-open step must sit at the known layout",
		},
		{
			name:     "in-layout step numbered after its bump",
			steps:    []Step{newStep(1, "a", PreOpen, 0, 1), newStep(2, "b", PreOpen, 0, 0)},
			fragment: "must precede the bump",
		},
		{
			name:     "in-layout step beyond the known layout",
			steps:    []Step{newStep(1, "a", PreOpen, 5, 5)},
			fragment: "beyond the known layout",
		},
		{
			name:     "missing plan",
			steps:    []Step{{Number: 1, Name: "a", Phase: PreOpen}},
			fragment: "missing plan",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewCatalog(tc.steps...)

			require.ErrorContains(t, err, tc.fragment)
			require.ErrorContains(t, err, "migrate:")
			assert.Nil(t, c)
		})
	}
}

func TestCatalog_OrderRunsInLayoutOrderThenPostOpen(t *testing.T) {
	c, err := NewCatalog(chain()...)
	require.NoError(t, err)

	assert.Equal(t, []int{1, 2, 3, 4}, numbers(c.order(0, neverApplied)))

	appliedThree := func(s Step) bool { return s.Number == 3 }
	assert.Equal(t, []int{4}, numbers(c.order(1, appliedThree)))
}

func TestCatalog_OrderSkipsStepsBelowTheLayout(t *testing.T) {
	c, err := NewCatalog(chain()...)
	require.NoError(t, err)

	assert.Equal(t, []int{3, 4}, numbers(c.order(1, neverApplied)))
}

func TestCatalog_OrderOmitsAppliedInLayoutSteps(t *testing.T) {
	c, err := NewCatalog(chain()...)
	require.NoError(t, err)

	appliedOne := func(s Step) bool { return s.Number == 1 }

	assert.Equal(t, []int{2, 3, 4}, numbers(c.order(0, appliedOne)))
}

func TestCatalog_OrderDecidesABumpByLayoutAlone(t *testing.T) {
	c, err := NewCatalog(chain()...)
	require.NoError(t, err)

	everythingApplied := func(Step) bool { return true }

	assert.Equal(t, []int{2}, numbers(c.order(0, everythingApplied)))
	assert.Empty(t, c.order(1, everythingApplied))
}

func TestCatalog_OrderWalksEveryLayoutFromTheCurrentOne(t *testing.T) {
	c, err := NewCatalog(
		newStep(1, "a", PreOpen, 0, 1),
		newStep(2, "b", PreOpen, 1, 1),
		newStep(3, "c", PreOpen, 1, 2),
		newStep(4, "d", PreOpen, 2, 2),
	)
	require.NoError(t, err)

	assert.Equal(t, []int{1, 2, 3, 4}, numbers(c.order(0, neverApplied)))
	assert.Equal(t, []int{2, 3, 4}, numbers(c.order(1, neverApplied)))
	assert.Equal(t, []int{4}, numbers(c.order(2, neverApplied)))

	t.Run("layout order wins over number order", func(t *testing.T) {
		c, err := NewCatalog(
			newStep(1, "b", PreOpen, 1, 2),
			newStep(2, "a", PreOpen, 0, 1),
		)
		require.NoError(t, err)

		assert.Equal(t, []int{2, 1}, numbers(c.order(0, neverApplied)))
	})
}

func TestCatalog_OrderIgnoresTheOrderStepsWereGiven(t *testing.T) {
	given := chain()
	given[0], given[3] = given[3], given[0]
	c, err := NewCatalog(given...)
	require.NoError(t, err)

	assert.Equal(t, []int{1, 2, 3, 4}, numbers(c.order(0, neverApplied)))
	assert.Equal(t, 4, given[0].Number)
}
