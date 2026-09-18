//go:build integration

package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wsusecase "github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
)

// A fork from before OwnsWorkspace existed replays with the flag unset (asynx
// stores RFC 6902 patches; the chat aggregate has no upcaster), and the
// conversation that forked it has been chatted in, so it carries a title.
// ResolveOwningChat's legacy fallback drops every titled row, so the first
// GET .../workspaces mints a brand-new empty owner (or elects an untitled
// thread) over the real one: the sidebar then draws the fork as an
// "Untitled chat" branch row and files the user's conversation under it as
// a thread, and every chat-keyed verb on the branch addresses the empty chat.
func TestRegression_LegacyForkKeepsItsTitledConversationAsOwner(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	ctx := context.Background()

	repo, err := h.app.GORM.Repositories.FindByKey(ctx, imported.repoID)
	require.NoError(t, err)
	parent, err := h.app.Repositories.Workspace.Get(ctx, imported.workspaceID)
	require.NoError(t, err)
	own := true
	fork, err := h.app.Usecases.Workspace.CreateChild(ctx, wsusecase.CreateChildInput{
		RepoID:       imported.repoID,
		ProjectID:    imported.projectID,
		RepoPath:     repo.Path,
		RemoteURL:    repo.RemoteURL,
		Branch:       "feat/legacy-fork",
		ParentID:     imported.workspaceID,
		ParentBranch: parent.Branch,
		OwnWorktree:  &own,
	})
	require.NoError(t, err)
	// The legacy owner: anchored to the fork at creation, ownership never
	// recorded (Create writes WorkspaceID without OwnsWorkspace), titled by
	// the turns it hosted.
	conv, err := h.app.Usecases.AgentChat.MintChat(ctx, fork.ID)
	require.NoError(t, err)
	require.NoError(t, h.app.Usecases.AgentChat.RenameChat(ctx, conv, "Fix login bug", "agent"))
	h.Quiesce()

	row := workspaceRow(t, h, imported, fork.ID)
	assert.Equal(t, conv, row.OwningChatID,
		"the conversation that forked the branch must stay its owner; got %q", row.OwningChatID)

	// And the fork's row must not have grown a second, empty chat.
	var rows []agentChatDTO
	h.get(repoBase(imported)+"/chats", &rows)
	for _, r := range rows {
		if r.WorkspaceID == fork.ID && r.ID != conv {
			t.Errorf("a fresh chat %s (title %q) was minted over the legacy owner", r.ID, r.Title)
		}
	}
}
