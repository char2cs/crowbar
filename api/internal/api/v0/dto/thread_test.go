package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestThreadDTOFrom(t *testing.T) {
	created := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	replied := created.Add(time.Minute)
	rt := domain.ReviewThread{
		ID:         "t1",
		WsID:       "w1",
		FilePath:   "main.go",
		LineNumber: 12,
		Side:       domain.ReviewSideRight,
		Status:     domain.ReviewThreadStatusResolved,
		CreatedAt:  created,
		Messages: []domain.ReviewMessage{
			{ID: "m1", Author: "alice", Body: "root comment", CreatedAt: created},
			{ID: "m2", Author: "bob", Body: "a reply", CreatedAt: replied},
		},
	}

	got := dto.ThreadDTOFrom(rt, "p1", "r1")

	assert.Equal(t, "t1", got.ID)
	assert.Equal(t, "p1", got.ProjectID)
	assert.Equal(t, "r1", got.RepoID)
	assert.Equal(t, "w1", got.WorkspaceID)
	assert.Equal(t, "main.go", got.FilePath)
	assert.Equal(t, 12, got.Line)
	assert.Equal(t, "right", got.Side)
	assert.Equal(t, "root comment", got.Body)
	assert.Equal(t, "alice", got.Author)
	assert.Equal(t, "m1", got.MessageID)
	assert.True(t, got.Resolved)
	assert.False(t, got.Deleted)
	assert.Equal(t, created, got.CreatedAt)

	require.Len(t, got.Replies, 1)
	assert.Equal(t, "m2", got.Replies[0].ID)
	assert.Equal(t, "t1", got.Replies[0].ThreadID)
	assert.Equal(t, "a reply", got.Replies[0].Body)
	assert.Equal(t, "bob", got.Replies[0].Author)
	assert.Equal(t, replied, got.Replies[0].CreatedAt)
}

func TestThreadDTOFrom_NoMessages(t *testing.T) {
	rt := domain.ReviewThread{ID: "t2", WsID: "w1", FilePath: "x.go", Side: domain.ReviewSideLeft}

	got := dto.ThreadDTOFrom(rt, "p1", "r1")

	assert.Empty(t, got.Body)
	assert.Empty(t, got.Author)
	assert.NotNil(t, got.Replies)
	assert.Empty(t, got.Replies)
}

func TestThreadDTOFrom_Unresolved(t *testing.T) {
	rt := domain.ReviewThread{
		ID:     "t3",
		WsID:   "w1",
		Status: domain.ReviewThreadStatusOpen,
		Messages: []domain.ReviewMessage{
			{ID: "m1", Body: "only root", CreatedAt: time.Now()},
		},
	}

	got := dto.ThreadDTOFrom(rt, "p1", "r1")

	assert.False(t, got.Resolved)
	assert.Empty(t, got.Replies)
}

func TestThreadDTOList(t *testing.T) {
	threads := []domain.ReviewThread{
		{ID: "t1", WsID: "w1", Messages: []domain.ReviewMessage{{ID: "m1", Body: "a"}}},
		{ID: "t2", WsID: "w1", Messages: []domain.ReviewMessage{{ID: "m2", Body: "b"}}},
	}

	got := dto.ThreadDTOList(threads, "p1", "r1")

	require.Len(t, got, 2)
	assert.Equal(t, "t1", got[0].ID)
	assert.Equal(t, "p1", got[0].ProjectID)
	assert.Equal(t, "r1", got[1].RepoID)

	empty := dto.ThreadDTOList(nil, "p1", "r1")
	assert.NotNil(t, empty)
	assert.Empty(t, empty)
}

func TestThreadDTOFrom_CarriesAgentAttributionAtBothLevels(t *testing.T) {
	created := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	rt := domain.ReviewThread{
		ID:        "t1",
		WsID:      "w1",
		FilePath:  "src/auth.go",
		Side:      domain.ReviewSideRight,
		Status:    domain.ReviewThreadStatusOpen,
		CreatedAt: created,
		Messages: []domain.ReviewMessage{
			{
				ID: "m1", Author: "claude", IsAgent: true,
				ProviderID: "claude", ChatID: "chat-1",
				Body: "This leaks the token.", CreatedAt: created,
			},
			{
				ID: "m2", Author: "codex", IsAgent: true,
				ProviderID: "codex", ChatID: "chat-2",
				Body: "Bounded it.", CreatedAt: created.Add(time.Minute),
			},
			{ID: "m3", Body: "thanks", CreatedAt: created.Add(2 * time.Minute)},
		},
	}

	got := dto.ThreadDTOFrom(rt, "p1", "r1")

	assert.Equal(t, "claude", got.ProviderID)
	assert.Equal(t, "chat-1", got.ChatID)

	require.Len(t, got.Replies, 2)
	assert.Equal(t, "codex", got.Replies[0].ProviderID)
	assert.Equal(t, "chat-2", got.Replies[0].ChatID)
	assert.Empty(t, got.Replies[1].ProviderID, "a human reply names no provider")
	assert.Empty(t, got.Replies[1].ChatID, "a human reply names no chat")
}

func TestThreadDTOFrom_PreAttributionThreadOmitsTheFields(t *testing.T) {
	rt := domain.ReviewThread{
		ID:   "t1",
		WsID: "w1",
		Messages: []domain.ReviewMessage{
			{ID: "m1", Author: "claude", IsAgent: true, Body: "old finding"},
			{ID: "m2", Body: "old reply"},
		},
	}

	got := dto.ThreadDTOFrom(rt, "p1", "r1")
	assert.Empty(t, got.ProviderID)
	assert.Empty(t, got.ChatID)
	require.Len(t, got.Replies, 1)
	assert.Empty(t, got.Replies[0].ProviderID)

	out, err := json.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "providerId")
	assert.NotContains(t, string(out), "chatId")
	assert.True(t, got.IsAgent)
}
