package agentchat_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestAgentChat_SaveAndListSegments(t *testing.T) {
	db, err := storesqlite.OpenDB(filepath.Join(t.TempDir(), "v.db"))
	require.NoError(t, err)
	repo, err := agentchat.New(db)
	require.NoError(t, err)
	ctx := context.Background()

	require.NoError(t, repo.SaveChat(ctx, domain.AgentChat{ID: "c1", WorkspaceID: "w1", CreatedAt: time.Now()}))
	require.NoError(t, repo.SaveSegment(ctx, domain.AgentSegment{ID: "s1", ChatID: "c1", ProviderID: "claude", ProviderSessionID: "sid-1", Status: "active"}))
	require.NoError(t, repo.SaveSegment(ctx, domain.AgentSegment{ID: "s2", ChatID: "c1", ProviderID: "codex", ProviderSessionID: "sid-2", Status: "active"}))

	got, err := repo.ListSegmentsByChat(ctx, "c1")
	require.NoError(t, err)
	require.Len(t, got, 2)

	chat, err := repo.GetChat(ctx, "c1")
	require.NoError(t, err)
	require.Equal(t, "w1", chat.WorkspaceID)

	_, err = repo.GetChat(ctx, "missing")
	require.ErrorIs(t, err, agentchat.ErrNotFound)
}
