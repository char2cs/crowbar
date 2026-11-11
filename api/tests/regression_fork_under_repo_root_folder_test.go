//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A folder started on the repo header is filed at the bare root (parentId
// ""), with no workspace-owning row above it. Fork on that folder — and
// "Make workspace" on a thread inside it — walked to "" and was refused with
// no fork parent; the folder applies its parent's logic, and its parent is the
// repo header, i.e. the repo's default checkout.
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
