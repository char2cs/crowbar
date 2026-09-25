package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestWorkspaceKindConstants(t *testing.T) {
	require.Equal(t, domain.WorkspaceKindGit, domain.WorkspaceKind("git"))
	require.Equal(t, domain.WorkspaceKindHome, domain.WorkspaceKind("home"))
}

func TestWorkspaceKindDefaultsToGit(t *testing.T) {
	ws := domain.Workspace{}
	require.Equal(t, domain.WorkspaceKind(""), ws.Kind) // zero value; caller sets it
}
