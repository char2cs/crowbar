package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// The owner is the row that RECORDS ownership. A thread started inside the
// workspace shares its WorkspaceID and is older or titled or filed under it —
// none of that makes it the owner, and no heuristic may elect it.
func TestResolveOwningChat(t *testing.T) {
	t.Run("no rows resolves nothing", func(t *testing.T) {
		_, ok := domain.ResolveOwningChat(nil)
		require.False(t, ok)
	})

	t.Run("rows that record no ownership resolve nothing", func(t *testing.T) {
		_, ok := domain.ResolveOwningChat([]domain.Chat{
			{ID: "thread", CreatedAt: time.Unix(0, 0).UTC()},
			{ID: "branch", Type: domain.ChatTypeBranch},
		})
		require.False(t, ok)
	})

	t.Run("the recorded owner wins over an older thread", func(t *testing.T) {
		thread := domain.Chat{ID: "thread", CreatedAt: time.Unix(0, 0).UTC()}
		owner := domain.Chat{ID: "owner", OwnsWorkspace: true, CreatedAt: time.Unix(100, 0).UTC()}
		got, ok := domain.ResolveOwningChat([]domain.Chat{thread, owner})
		require.True(t, ok)
		assert.Equal(t, "owner", got.ID)
	})

	t.Run("two recorded owners resolve to the earliest, whatever the order", func(t *testing.T) {
		older := domain.Chat{ID: "z-older", OwnsWorkspace: true, CreatedAt: time.Unix(0, 0).UTC()}
		newer := domain.Chat{ID: "a-newer", OwnsWorkspace: true, CreatedAt: time.Unix(100, 0).UTC()}
		for _, rows := range [][]domain.Chat{{older, newer}, {newer, older}} {
			got, ok := domain.ResolveOwningChat(rows)
			require.True(t, ok)
			assert.Equal(t, "z-older", got.ID)
		}
	})
}
