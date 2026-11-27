package project_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
)

func newOwnedImportDeps(
	t *testing.T,
	projects *mocks.ProjectStore,
	ws *mocks.WorkspaceRepo,
) project.ImportDeps {
	t.Helper()
	return project.ImportDeps{
		CrowbarHome: tempCrowbarHome(t),
		Projects:    projects,
		Repos:       mocks.NewRepositoryStore(),
		Workspaces:  ws,
		Git:         mocks.NewGitEngine(),
		Provider:    mocks.NewProviderEngine(),
		Discover:    func(string, int) ([]string, error) { return nil, nil },
		RefRunner:   noRefRunner,
		Nodes:       mocks.NewNodePlacements(),
		Now:         func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat:        statExists,
	}
}

func TestImport_NoOwningChatsWired_RefusesAndPersistsNothing(t *testing.T) {
	projects := mocks.NewProjectStore()
	ws := mocks.NewWorkspaceRepo()
	uc := project.NewImport(newOwnedImportDeps(t, projects, ws))

	_, err := uc.Import(context.Background(), "P", "/root")

	require.ErrorIs(t, err, project.ErrNoOwningChats)
	assert.Empty(t, ws.Created, "no workspace may be born without the chat that owns it")
	assert.Empty(t, projects.Saved, "the project row rolls back with the home that failed")
}

func TestImport_MintOwningChatFails_WritesNoWorkspaceRow(t *testing.T) {
	projects := mocks.NewProjectStore()
	ws := mocks.NewWorkspaceRepo()
	chats := newFakeOwningChats()
	chats.mintErr = errors.New("mint boom")
	uc := project.NewImport(newOwnedImportDeps(t, projects, ws))
	uc.SetOwningChats(chats)

	_, err := uc.Import(context.Background(), "P", "/root")

	require.Error(t, err)
	assert.Empty(t, ws.Created, "the row is never reached once the chat cannot be minted")
	assert.Empty(t, ws.Deleted, "nothing was created, so nothing is taken back out")
	assert.Empty(t, chats.discards(), "an unminted chat is not a chat to discard")
}

func TestImport_AttachOwningWorkspaceFails_TakesBothHalvesBackOut(t *testing.T) {
	projects := mocks.NewProjectStore()
	ws := mocks.NewWorkspaceRepo()
	chats := newFakeOwningChats()
	chats.attachErr = errors.New("attach boom")
	uc := project.NewImport(newOwnedImportDeps(t, projects, ws))
	uc.SetOwningChats(chats)

	_, err := uc.Import(context.Background(), "P", "/root")

	require.Error(t, err)
	require.Len(t, ws.Created, 1, "the row was written before the attach was attempted")
	assert.Equal(t, []string{ws.Created[0].ID}, ws.Deleted,
		"the workspace no chat came to own must be taken back out")
	assert.Equal(t, 1, chats.mintedCount())
	assert.Len(t, chats.discards(), 1, "the chat minted for it goes too")
	assert.Empty(t, projects.Saved, "the project row rolls back with the home that failed")
}

func TestImport_AttachFails_WorkspaceDeleteAlsoFails_StillSurfacesTheCause(t *testing.T) {
	projects := mocks.NewProjectStore()
	ws := mocks.NewWorkspaceRepo()
	ws.DeleteErr = errors.New("delete boom")
	chats := newFakeOwningChats()
	chats.attachErr = errors.New("attach boom")
	uc := project.NewImport(newOwnedImportDeps(t, projects, ws))
	uc.SetOwningChats(chats)

	_, err := uc.Import(context.Background(), "P", "/root")

	require.ErrorContains(t, err, "attach boom",
		"a failed cleanup must never replace the cause of the failure")
	assert.Len(t, chats.discards(), 1, "the chat is still discarded even when the row could not be")
}

func TestImport_HomeWorkspace_IsOwnedByTheChatMintedForIt(t *testing.T) {
	projects := mocks.NewProjectStore()
	ws := mocks.NewWorkspaceRepo()
	chats := newFakeOwningChats()
	uc := project.NewImport(newOwnedImportDeps(t, projects, ws))
	uc.SetOwningChats(chats)

	_, err := uc.Import(context.Background(), "P", "/root")
	require.NoError(t, err)

	require.Len(t, ws.Created, 1)
	owner, ok := chats.ownerOf(ws.Created[0].ID)
	require.True(t, ok, "the created workspace must be owned by a chat")
	assert.NotEmpty(t, owner)
	assert.Empty(t, ws.Deleted, "a clean create tombstones nothing")
	assert.Empty(t, chats.discards(), "a clean create discards no chat")
}
