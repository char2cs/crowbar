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

// newOwnedImportDeps builds the smallest import that creates exactly ONE
// workspace — the project-level home, discovering no repo at all — so a test can
// watch the chat side of that single create with no repo-tree noise around it.
func newOwnedImportDeps(
	projects *mocks.ProjectStore,
	ws *mocks.WorkspaceRepo,
) project.ImportDeps {
	return project.ImportDeps{
		Projects:   projects,
		Repos:      mocks.NewRepositoryStore(),
		Workspaces: ws,
		Git:        mocks.NewGitEngine(),
		Provider:   mocks.NewProviderEngine(),
		Discover:   func(string, int) ([]string, error) { return nil, nil },
		RefRunner:  noRefRunner,
		Now:        func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat:       statExists,
	}
}

// TestImport_NoOwningChatsWired_RefusesAndPersistsNothing is the guard itself: a
// usecase whose chat side was never wired must refuse at its first create rather
// than fall back to the workspace-only create it used to do.
//
// It constructs project.NewImport DIRECTLY — the one test in this package that
// must NOT go through newImportUsecase, whose whole job is to wire the surface
// this test needs absent.
func TestImport_NoOwningChatsWired_RefusesAndPersistsNothing(t *testing.T) {
	projects := mocks.NewProjectStore()
	ws := mocks.NewWorkspaceRepo()
	uc := project.NewImport(newOwnedImportDeps(projects, ws))

	_, err := uc.Import(context.Background(), "P", "/root")

	require.ErrorIs(t, err, project.ErrNoOwningChats)
	assert.Empty(t, ws.Created, "no workspace may be born without the chat that owns it")
	assert.Empty(t, projects.Saved, "the project row rolls back with the home that failed")
}

// TestImport_MintOwningChatFails_WritesNoWorkspaceRow proves the mint runs BEFORE
// the row: when the chat cannot be minted there is nothing to roll back, because
// the workspace was never created in the first place.
func TestImport_MintOwningChatFails_WritesNoWorkspaceRow(t *testing.T) {
	projects := mocks.NewProjectStore()
	ws := mocks.NewWorkspaceRepo()
	chats := newFakeOwningChats()
	chats.mintErr = errors.New("mint boom")
	uc := project.NewImport(newOwnedImportDeps(projects, ws))
	uc.SetOwningChats(chats)

	_, err := uc.Import(context.Background(), "P", "/root")

	require.Error(t, err)
	assert.Empty(t, ws.Created, "the row is never reached once the chat cannot be minted")
	assert.Empty(t, ws.Deleted, "nothing was created, so nothing is taken back out")
	assert.Empty(t, chats.discards(), "an unminted chat is not a chat to discard")
}

// TestImport_AttachOwningWorkspaceFails_TakesBothHalvesBackOut covers the
// rollback that makes an orphan unrepresentable: the row WAS written, then could
// not be pointed at the chat minted for it, so the workspace is tombstoned and
// the chat discarded. A row left behind here is the exact orphan the guard
// exists to prevent.
func TestImport_AttachOwningWorkspaceFails_TakesBothHalvesBackOut(t *testing.T) {
	projects := mocks.NewProjectStore()
	ws := mocks.NewWorkspaceRepo()
	chats := newFakeOwningChats()
	chats.attachErr = errors.New("attach boom")
	uc := project.NewImport(newOwnedImportDeps(projects, ws))
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

// TestImport_AttachFails_WorkspaceDeleteAlsoFails_StillSurfacesTheCause proves
// the rollback is best-effort and never masks what actually went wrong: a
// tombstone that itself fails is logged, and the attach failure is still the
// error the caller sees.
func TestImport_AttachFails_WorkspaceDeleteAlsoFails_StillSurfacesTheCause(t *testing.T) {
	projects := mocks.NewProjectStore()
	ws := mocks.NewWorkspaceRepo()
	ws.DeleteErr = errors.New("delete boom")
	chats := newFakeOwningChats()
	chats.attachErr = errors.New("attach boom")
	uc := project.NewImport(newOwnedImportDeps(projects, ws))
	uc.SetOwningChats(chats)

	_, err := uc.Import(context.Background(), "P", "/root")

	require.ErrorContains(t, err, "attach boom",
		"a failed cleanup must never replace the cause of the failure")
	assert.Len(t, chats.discards(), 1, "the chat is still discarded even when the row could not be")
}

// TestImport_HomeWorkspace_IsOwnedByTheChatMintedForIt is the success half: the
// workspace a clean import creates ends up attached to a chat that was minted
// for it, which is the whole point of routing every create through
// createOwnedWorkspace.
func TestImport_HomeWorkspace_IsOwnedByTheChatMintedForIt(t *testing.T) {
	projects := mocks.NewProjectStore()
	ws := mocks.NewWorkspaceRepo()
	chats := newFakeOwningChats()
	uc := project.NewImport(newOwnedImportDeps(projects, ws))
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
