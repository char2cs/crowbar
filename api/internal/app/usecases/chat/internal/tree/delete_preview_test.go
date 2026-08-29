package tree_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// newUsecaseWithGitStatus is newUsecase with the fake WorkspaceGitStatus port
// exposed, for the DeletePreview tests below that seed a workspace's file
// counts.
func newUsecaseWithGitStatus(
	t *testing.T,
) (*mocks.AgentChatPlacements, tree.Usecase, *mocks.AgentWorkspaceGitStatus) {
	t.Helper()
	chats := mocks.NewAgentChatPlacements()
	git := mocks.NewAgentWorkspaceGitStatus()
	work := inflight.NewWork()
	return chats, tree.New(chats, chats, work, git), git
}

// A folder's subtree can span more than one independent workspace (the whole
// reason this route exists — see the backend addendum spec §1), so the file
// count has to be a real sum across every one of them, not a single
// workspace's own count.
func TestDeletePreview_SumsFileCountsAcrossSubtree(t *testing.T) {
	chats, uc, git := newUsecaseWithGitStatus(t)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "folder-1", Type: domain.ChatTypeFolder},
		domain.Chat{ID: "chat-a", Type: domain.ChatTypeChat, ParentID: "folder-1", WorkspaceID: "ws-a"},
		domain.Chat{ID: "chat-b", Type: domain.ChatTypeChat, ParentID: "folder-1", WorkspaceID: "ws-b"},
	)
	git.Set("ws-a", 3, 1)
	git.Set("ws-b", 2, 0)

	chatCount, fileCount, err := uc.DeletePreview(context.Background(), "folder-1")
	require.NoError(t, err)
	assert.Equal(t, 2, chatCount, "the folder itself is not a chat")
	assert.Equal(t, 6, fileCount, "3+1+2+0")
}

// A CHAT root's own preview includes itself, not merely its descendants: a
// chat that owns a workspace is exactly the row DeleteChat would purge first.
func TestDeletePreview_ChatRootIncludesItself(t *testing.T) {
	chats, uc, git := newUsecaseWithGitStatus(t)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: "ws-1"},
	)
	git.Set("ws-1", 5, 4)

	chatCount, fileCount, err := uc.DeletePreview(context.Background(), "c1")
	require.NoError(t, err)
	assert.Equal(t, 1, chatCount)
	assert.Equal(t, 9, fileCount)
}

// A thread inherits its cwd from an ancestor and owns no workspace of its
// own — WorkspaceID is empty — so it counts toward chatCount but contributes
// nothing to fileCount; the ancestor it reads is the only row asked for a
// git status.
func TestDeletePreview_ThreadsWithNoWorkspaceContributeNoFiles(t *testing.T) {
	chats, uc, git := newUsecaseWithGitStatus(t)
	chats.Rows = append(chats.Rows,
		domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: "ws-1"},
		domain.Chat{ID: "c2", Type: domain.ChatTypeChat, ParentID: "c1"},
	)
	git.Set("ws-1", 5, 4)

	chatCount, fileCount, err := uc.DeletePreview(context.Background(), "c1")
	require.NoError(t, err)
	assert.Equal(t, 2, chatCount)
	assert.Equal(t, 9, fileCount)
}

// A folder holds no workspace of its own (see the tree package doc), so a
// bare folder subtree with nothing planted in it previews as zero and zero.
func TestDeletePreview_EmptyFolderIsZeroAndZero(t *testing.T) {
	chats, uc, _ := newUsecaseWithGitStatus(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "folder-1", Type: domain.ChatTypeFolder})

	chatCount, fileCount, err := uc.DeletePreview(context.Background(), "folder-1")
	require.NoError(t, err)
	assert.Equal(t, 0, chatCount)
	assert.Equal(t, 0, fileCount)
}

func TestDeletePreview_UnknownIDIsNotFound(t *testing.T) {
	_, uc, _ := newUsecaseWithGitStatus(t)

	_, _, err := uc.DeletePreview(context.Background(), "nowhere")
	assert.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestDeletePreview_SurfacesASnapshotFailure(t *testing.T) {
	chats, uc, _ := newUsecaseWithGitStatus(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: "ws-1"})
	chats.ListErr = errors.New("folders down")

	_, _, err := uc.DeletePreview(context.Background(), "c1")
	assert.ErrorContains(t, err, "folders down")
}

// A git-status failure on any workspace-owning row must surface rather than
// silently under-report the count a delete confirm is about to show.
func TestDeletePreview_SurfacesAGitStatusError(t *testing.T) {
	chats, uc, git := newUsecaseWithGitStatus(t)
	chats.Rows = append(chats.Rows, domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: "ws-1"})
	git.Err = errors.New("git down")

	_, _, err := uc.DeletePreview(context.Background(), "c1")
	assert.ErrorContains(t, err, "git down")
}
