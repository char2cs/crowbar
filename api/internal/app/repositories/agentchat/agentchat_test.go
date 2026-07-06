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

func TestAgentChat_GetActiveSegmentByCrowbarID(t *testing.T) {
	db, err := storesqlite.OpenDB(filepath.Join(t.TempDir(), "v.db"))
	require.NoError(t, err)
	repo, err := agentchat.New(db)
	require.NoError(t, err)
	ctx := context.Background()

	now := time.Now()
	later := now.Add(1 * time.Hour)

	// Save chat
	require.NoError(t, repo.SaveChat(ctx, domain.AgentChat{ID: "c1", WorkspaceID: "w1", CreatedAt: now}))

	// Save two segments with the same CrowbarSegmentID: one active, one moved
	require.NoError(t, repo.SaveSegment(ctx, domain.AgentSegment{
		ID:               "s1",
		ChatID:           "c1",
		CrowbarSegmentID: "csi-1",
		ProviderID:       "claude",
		ProviderSessionID: "sid-1",
		Status:           "moved",
		StartedAt:        now,
	}))
	require.NoError(t, repo.SaveSegment(ctx, domain.AgentSegment{
		ID:               "s2",
		ChatID:           "c1",
		CrowbarSegmentID: "csi-1",
		ProviderID:       "claude",
		ProviderSessionID: "sid-2",
		Status:           "active",
		StartedAt:        later,
	}))

	// GetActiveSegmentByCrowbarID should return the active segment (s2)
	got, err := repo.GetActiveSegmentByCrowbarID(ctx, "csi-1")
	require.NoError(t, err)
	require.Equal(t, "s2", got.ID)
	require.Equal(t, "active", got.Status)

	// ErrNotFound when no active segment exists for a crowbar id
	_, err = repo.GetActiveSegmentByCrowbarID(ctx, "nonexistent")
	require.ErrorIs(t, err, agentchat.ErrNotFound)
}

func TestAgentChat_GetSegment(t *testing.T) {
	db, err := storesqlite.OpenDB(filepath.Join(t.TempDir(), "v.db"))
	require.NoError(t, err)
	repo, err := agentchat.New(db)
	require.NoError(t, err)
	ctx := context.Background()

	now := time.Now()

	// Save chat and segment
	require.NoError(t, repo.SaveChat(ctx, domain.AgentChat{ID: "c1", WorkspaceID: "w1", CreatedAt: now}))
	seg := domain.AgentSegment{
		ID:               "s1",
		ChatID:           "c1",
		CrowbarSegmentID: "csi-1",
		ProviderID:       "claude",
		ProviderSessionID: "sid-1",
		Status:           "active",
		StartedAt:        now,
	}
	require.NoError(t, repo.SaveSegment(ctx, seg))

	// Get segment by id
	got, err := repo.GetSegment(ctx, "s1")
	require.NoError(t, err)
	require.Equal(t, "s1", got.ID)
	require.Equal(t, "c1", got.ChatID)
	require.Equal(t, "claude", got.ProviderID)

	// ErrNotFound for missing id
	_, err = repo.GetSegment(ctx, "missing")
	require.ErrorIs(t, err, agentchat.ErrNotFound)
}

func TestAgentChat_AllSegments(t *testing.T) {
	db, err := storesqlite.OpenDB(filepath.Join(t.TempDir(), "v.db"))
	require.NoError(t, err)
	repo, err := agentchat.New(db)
	require.NoError(t, err)
	ctx := context.Background()

	now := time.Now()

	// Save two chats
	require.NoError(t, repo.SaveChat(ctx, domain.AgentChat{ID: "c1", WorkspaceID: "w1", CreatedAt: now}))
	require.NoError(t, repo.SaveChat(ctx, domain.AgentChat{ID: "c2", WorkspaceID: "w1", CreatedAt: now}))

	// Save segments across both chats
	require.NoError(t, repo.SaveSegment(ctx, domain.AgentSegment{
		ID:                "s1",
		ChatID:            "c1",
		CrowbarSegmentID:  "csi-1",
		ProviderID:        "claude",
		ProviderSessionID: "sid-1",
		Status:            "active",
		StartedAt:         now,
	}))
	require.NoError(t, repo.SaveSegment(ctx, domain.AgentSegment{
		ID:                "s2",
		ChatID:            "c1",
		CrowbarSegmentID:  "csi-2",
		ProviderID:        "claude",
		ProviderSessionID: "sid-2",
		Status:            "active",
		StartedAt:         now,
	}))
	require.NoError(t, repo.SaveSegment(ctx, domain.AgentSegment{
		ID:                "s3",
		ChatID:            "c2",
		CrowbarSegmentID:  "csi-3",
		ProviderID:        "codex",
		ProviderSessionID: "sid-3",
		Status:            "active",
		StartedAt:         now,
	}))

	// AllSegments should return all three segments
	got, err := repo.AllSegments(ctx)
	require.NoError(t, err)
	require.Len(t, got, 3)

	// Verify all segment IDs are present
	ids := make(map[string]bool)
	for _, seg := range got {
		ids[seg.ID] = true
	}
	require.True(t, ids["s1"])
	require.True(t, ids["s2"])
	require.True(t, ids["s3"])
}

func TestAgentChat_ListChats(t *testing.T) {
	db, err := storesqlite.OpenDB(filepath.Join(t.TempDir(), "v.db"))
	require.NoError(t, err)
	repo, err := agentchat.New(db)
	require.NoError(t, err)
	ctx := context.Background()

	now := time.Now()

	// Save two chats
	require.NoError(t, repo.SaveChat(ctx, domain.AgentChat{
		ID:          "c1",
		WorkspaceID: "w1",
		Title:       "Chat 1",
		CreatedAt:   now,
	}))
	require.NoError(t, repo.SaveChat(ctx, domain.AgentChat{
		ID:          "c2",
		WorkspaceID: "w1",
		Title:       "Chat 2",
		CreatedAt:   now,
	}))

	// ListChats should return both chats
	got, err := repo.ListChats(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2)

	// Verify both chat IDs are present
	ids := make(map[string]bool)
	for _, chat := range got {
		ids[chat.ID] = true
	}
	require.True(t, ids["c1"])
	require.True(t, ids["c2"])
}
