package migrate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlan_Empty(t *testing.T) {
	assert.True(t, Plan{}.Empty())
	assert.True(t, Plan{RewritesState: true, NeedsGit: true}.Empty())

	withAction := Plan{Actions: []Action{{Kind: Delete, Subject: "state/upgrades", Count: Unknown}}}
	assert.False(t, withAction.Empty())
}
