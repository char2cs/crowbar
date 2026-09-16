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
		owner, ok := domain.ResolveOwningChat(nil)
		require.False(t, ok)
		assert.Equal(t, domain.Chat{}, owner)
	})

	t.Run("a single row is the owner", func(t *testing.T) {
		row := domain.Chat{ID: "c1", Type: domain.ChatTypeChat}
		owner, ok := domain.ResolveOwningChat([]domain.Chat{row})
		require.True(t, ok)
		assert.Equal(t, row.ID, owner.ID)
	})

	t.Run("a legacy branch row wins over an older ordinary chat", func(t *testing.T) {
		legacy := domain.Chat{ID: "legacy", Type: domain.ChatTypeChat, CreatedAt: time.Unix(0, 0).UTC()}
		branch := domain.Chat{ID: "branch", Type: domain.ChatTypeBranch, CreatedAt: time.Unix(100, 0).UTC()}
		owner, ok := domain.ResolveOwningChat([]domain.Chat{legacy, branch})
		require.True(t, ok)
		assert.Equal(t, branch.ID, owner.ID, "the branch row must win regardless of row order")

		owner2, ok2 := domain.ResolveOwningChat([]domain.Chat{branch, legacy})
		require.True(t, ok2)
		assert.Equal(t, branch.ID, owner2.ID, "and the winner must not depend on which row came first")
	})

	t.Run("same-standing rows tiebreak by creation order", func(t *testing.T) {
		older := domain.Chat{ID: "z-older", Type: domain.ChatTypeChat, CreatedAt: time.Unix(0, 0).UTC()}
		newer := domain.Chat{ID: "a-newer", Type: domain.ChatTypeChat, CreatedAt: time.Unix(100, 0).UTC()}
		owner, ok := domain.ResolveOwningChat([]domain.Chat{newer, older})
		require.True(t, ok)
		assert.Equal(t, older.ID, owner.ID, "the earlier row wins, matching the rest of the tree's tiebreak")
	})
}
