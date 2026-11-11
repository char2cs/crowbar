package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

func TestPlacementTreeStep_PlanNamesWhatItWillConvert(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "state/events/"+LegacyNodeLog, "rows")
	before := testutil.TreeSnapshot(t, h)

	assert.Equal(t, migrate.Plan{
		Actions:       []migrate.Action{{Kind: migrate.Convert, Subject: "sidebar placements → project trees", Count: migrate.Unknown}},
		RewritesState: true,
	}, planOf(t, h, StepPlacementTree))
	assert.Equal(t, before, testutil.TreeSnapshot(t, h), "a plan writes nothing")
	assert.Equal(t, migrate.PostOpen, stepOf(t, StepPlacementTree).Phase, "it reads the open stores")
}

func TestPlacementTreeStep_AFreshHomeHasNothingToDo(t *testing.T) {
	h := testutil.FixtureHome(t)

	assert.True(t, planOf(t, h, StepPlacementTree).Empty())
}

// A home older than the position rows still has chat placements to copy, so
// the run writes the Tree and the plan asks for a backup.
func TestPlacementTreeStep_AHomeWithChatsButNoNodeLogStillRewritesState(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "state/events/"+legacyChatLog, "rows")

	assert.True(t, planOf(t, h, StepPlacementTree).RewritesState)
}
