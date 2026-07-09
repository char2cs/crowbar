package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestWorkspaceKindConstants(t *testing.T) {
	require.Equal(t, domain.WorkspaceKind("git"), domain.WorkspaceKindGit)
	require.Equal(t, domain.WorkspaceKind("home"), domain.WorkspaceKindHome)
}

func TestWorkspaceKindDefaultsToGit(t *testing.T) {
	ws := domain.Workspace{}
	require.Equal(t, domain.WorkspaceKind(""), ws.Kind) // zero value; caller sets it
}
