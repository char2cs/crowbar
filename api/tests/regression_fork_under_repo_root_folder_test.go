//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegression_ForkUnderRepoRootFolderForksTheDefaultWorkspace(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProjectHomeHoldsDefault(t, h)
	base := repoBase(imported)

	folder := createChatFolder(t, h, base, "root-folder", "")

	chatID, fork := forkFromSidebar(t, h, imported, folder.ID, "test/test")
	require.NotNil(t, fork)
	assert.Equal(t, imported.workspaceID, fork.ParentID, "the fork's git parent is the repo's default checkout")
	assert.Equal(t, "test/test", fork.Branch)
	assert.Equal(t, folder.ID, getAgentChat(t, h, base, chatID).ParentID)
}
