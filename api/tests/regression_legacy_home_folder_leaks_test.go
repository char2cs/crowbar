//go:build integration

package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A home folder written before Folder.HomeID existed (every home folder in
// the user's production data) still has HomeID "", and Folder.InHome treated
// "" as "visible from every home": it was listed by, counted by and renumbered
// through every project's home level, not only the one that created it. A
// read now adopts it into the home its own rows name — here, the chat filed
// inside it — and it is listed nowhere else.
func TestRegression_LegacyHomeFolderStillLeaksIntoEveryProject(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	projectA := createProjectForHome(t, h)
	projectB := createProjectForHome(t, h)
	baseA := "/v0/projects/" + projectA + "/home"
	baseB := "/v0/projects/" + projectB + "/home"

	legacy := createChatFolder(t, h, baseA, "legacy", "")
	filed := createHomeChatStub(t, h, projectA)
	h.patch(baseA+"/chats/"+filed+"/placement", map[string]any{"parentId": legacy.ID}, nil)
	h.Quiesce()

	ctx := context.Background()
	f, err := h.app.GORM.Folders.FindByKey(ctx, legacy.ID)
	require.NoError(t, err)
	require.NotNil(t, f)
	f.HomeID = ""
	require.NoError(t, h.app.GORM.Folders.Save(ctx, *f))

	inB := listChatFolders(t, h, baseB)
	_, leaked := chatFolderByID(inB, legacy.ID)
	t.Logf("project B's home folders: %+v", inB)
	assert.False(t, leaked, "a folder created in project A's home must not be listed by project B")

	inA := listChatFolders(t, h, baseA)
	_, kept := chatFolderByID(inA, legacy.ID)
	assert.True(t, kept, "project A's home still lists its own folder")
	adopted, err := h.app.GORM.Folders.FindByKey(ctx, legacy.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, adopted.HomeID, "the read records the home the folder belongs to")
}
