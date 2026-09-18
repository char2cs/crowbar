package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestResolveOwningChat pins the answer every wire-facing surface that still
// addresses a workspace's worktree through a chat id (chat-scoped HTTP
// routes, WorkspaceDTO.OwningChatID) needs: which candidate row, among
// everything ListChatsByWorkspace(workspaceID) returns, is the one to
// address. Moved here from the (now-deleted) chat-tree package's own
// owning_rows.go by 2026-09-08 sidebar-placement-unification Task 9, so it no
// longer depends on the boot-backfill machinery that used to maintain the
// rows it prefers.
func TestResolveOwningChat(t *testing.T) {
	t.Run("no rows resolves nothing", func(t *testing.T) {
		owner, ok := domain.ResolveOwningChat(nil, true)
		require.False(t, ok)
		assert.Equal(t, domain.Chat{}, owner)
	})

	t.Run("a single row is the owner", func(t *testing.T) {
		row := domain.Chat{ID: "c1", Type: domain.ChatTypeChat}
		owner, ok := domain.ResolveOwningChat([]domain.Chat{row}, true)
		require.True(t, ok)
		assert.Equal(t, row.ID, owner.ID)
	})

	t.Run("a legacy branch row wins over an older ordinary chat", func(t *testing.T) {
		legacy := domain.Chat{ID: "legacy", Type: domain.ChatTypeChat, CreatedAt: time.Unix(0, 0).UTC()}
		branch := domain.Chat{ID: "branch", Type: domain.ChatTypeBranch, CreatedAt: time.Unix(100, 0).UTC()}
		owner, ok := domain.ResolveOwningChat([]domain.Chat{legacy, branch}, true)
		require.True(t, ok)
		assert.Equal(t, branch.ID, owner.ID, "the branch row must win regardless of row order")

		owner2, ok2 := domain.ResolveOwningChat([]domain.Chat{branch, legacy}, true)
		require.True(t, ok2)
		assert.Equal(t, branch.ID, owner2.ID, "and the winner must not depend on which row came first")
	})

	t.Run("same-standing rows tiebreak by creation order", func(t *testing.T) {
		older := domain.Chat{ID: "z-older", Type: domain.ChatTypeChat, CreatedAt: time.Unix(0, 0).UTC()}
		newer := domain.Chat{ID: "a-newer", Type: domain.ChatTypeChat, CreatedAt: time.Unix(100, 0).UTC()}
		owner, ok := domain.ResolveOwningChat([]domain.Chat{newer, older}, true)
		require.True(t, ok)
		assert.Equal(t, older.ID, owner.ID, "the earlier row wins, matching the rest of the tree's tiebreak")
	})
}

// A thread started INSIDE a workspace shares its WorkspaceID with the owner,
// and in a workspace that had no owner yet the oldest-wins heuristic promoted
// that thread to owner — its row then folded into the branch/header row and
// vanished from the sidebar. Ownership is a recorded fact now.
func TestRegression_ResolveOwningChat_PrefersTheRecordedOwnerOverAnOlderThread(t *testing.T) {
	thread := domain.Chat{ID: "thread", Type: domain.ChatTypeChat, WorkspaceID: "ws", CreatedAt: time.Unix(0, 0).UTC()}
	owner := domain.Chat{ID: "owner", Type: domain.ChatTypeChat, WorkspaceID: "ws", OwnsWorkspace: true, CreatedAt: time.Unix(100, 0).UTC()}
	got, ok := domain.ResolveOwningChat([]domain.Chat{thread, owner}, false)
	require.True(t, ok)
	assert.Equal(t, "owner", got.ID)
}

func TestRegression_ResolveOwningChat_NeverPicksAThreadFiledUnderTheWorkspaceItself(t *testing.T) {
	thread := domain.Chat{ID: "thread", Type: domain.ChatTypeChat, WorkspaceID: "ws", ParentID: "ws"}
	_, ok := domain.ResolveOwningChat([]domain.Chat{thread}, false)
	assert.False(t, ok, "a thread under the workspace's own row is not its owner")
}

// A legacy home (owner never minted, or purged) holds only the user's own
// conversations. The oldest-wins fallback elected the earliest of them as the
// home's owner, and the sidebar hid it. A minted owner never carries a title;
// a conversation that has been chatted in does.
func TestRegression_ResolveOwningChat_NeverElectsATitledConversationAsLegacyOwner(t *testing.T) {
	c1 := domain.Chat{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: "home", Title: "plan the release", TitleLocked: true, CreatedAt: time.Unix(0, 0).UTC()}
	c2 := domain.Chat{ID: "c2", Type: domain.ChatTypeChat, WorkspaceID: "home", Title: "triage bugs", CreatedAt: time.Unix(100, 0).UTC()}
	_, ok := domain.ResolveOwningChat([]domain.Chat{c1, c2}, true)
	assert.False(t, ok, "a titled conversation is never a legacy owner")

	untitled := domain.Chat{ID: "u", Type: domain.ChatTypeChat, WorkspaceID: "home", CreatedAt: time.Unix(200, 0).UTC()}
	got, ok := domain.ResolveOwningChat([]domain.Chat{c1, untitled}, true)
	require.True(t, ok)
	assert.Equal(t, "u", got.ID, "an untitled legacy row still resolves")

	branch := domain.Chat{ID: "b", Type: domain.ChatTypeBranch, WorkspaceID: "home", Title: "main"}
	got, ok = domain.ResolveOwningChat([]domain.Chat{c1, branch}, true)
	require.True(t, ok)
	assert.Equal(t, "b", got.ID, "a legacy branch row keeps winning whatever its title")
}

// A fork minted before OwnsWorkspace existed replays with the flag unset (asynx
// stores RFC 6902 patches; no chat upcaster). Its owning conversation is the
// one that forked the branch and has been chatted in, so it carries a title —
// which the legacy fallback treated as "a user conversation, never an owner"
// on every ground alike. The fork then resolved to an untitled thread started inside it (or, with
// none, to nothing — EnsureOwner mints a fresh empty owner), demoting the
// real conversation to a thread under an "Untitled chat" branch row.
func TestRegression_ResolveOwningChat_LegacyTitledForkConversationStaysTheOwner(t *testing.T) {
	forkConv := domain.Chat{ID: "fork-conv", Type: domain.ChatTypeChat, WorkspaceID: "ws-fork", ParentID: "main-owner", Title: "Fix login bug", CreatedAt: time.Unix(0, 0).UTC()}
	thread := domain.Chat{ID: "thread", Type: domain.ChatTypeChat, WorkspaceID: "ws-fork", ParentID: "fork-conv", CreatedAt: time.Unix(100, 0).UTC()}

	got, ok := domain.ResolveOwningChat([]domain.Chat{forkConv, thread}, false)
	require.True(t, ok)
	assert.Equal(t, "fork-conv", got.ID, "the conversation that forked the branch owns it, not the thread under it")

	got, ok = domain.ResolveOwningChat([]domain.Chat{forkConv}, false)
	require.True(t, ok, "a titled legacy fork conversation alone must still resolve, or a fresh owner is minted over it")
	assert.Equal(t, "fork-conv", got.ID)
}
