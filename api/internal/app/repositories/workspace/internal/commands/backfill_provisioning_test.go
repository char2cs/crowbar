package commands

import (
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestBackfillProvisioning_RefusesARowThatAlreadyHasOne(t *testing.T) {
	cmd := BackfillProvisioning{ID: "w1"}
	require.ErrorIs(t, cmd.Validate(nil), asynxModels.ErrValidation)
	require.ErrorIs(t, cmd.Validate(&domain.Workspace{ID: "w1", Provisioning: domain.WorkspaceShared}),
		asynxModels.ErrValidation)
	assert.NoError(t, cmd.Validate(&domain.Workspace{ID: "w1"}))
}

func TestBackfillProvisioning_ReadsTheLegacyEncodingOnce(t *testing.T) {
	cases := map[string]struct {
		ws   domain.Workspace
		want domain.WorkspaceProvisioning
	}{
		"managed":      {domain.Workspace{WorktreePath: "/w"}, domain.WorkspaceProvisioned},
		"placeholder":  {domain.Workspace{HeldByPath: "/h"}, domain.WorkspacePlaceholder},
		"repo home":    {domain.Workspace{WorktreePath: "/r", IsDefault: true}, domain.WorkspaceShared},
		"project home": {domain.Workspace{WorktreePath: "/p", Kind: domain.WorkspaceKindHome}, domain.WorkspaceShared},
	}
	for name, c := range cases {
		got := BackfillProvisioning{ID: "w1"}.EmitEvent(&c.ws)
		assert.Equal(t, c.want, got.Provisioning, name)
	}
}
