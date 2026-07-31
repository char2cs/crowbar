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

// TestThreadDTOFrom asserts the converter maps the ReviewThread anchor
// (file/line/side), the root message (body/author/createdAt), the resolution
// flag, and the trailing messages into Replies, stamping the hierarchical
// project/repo scope passed by the caller (the ReviewThread aggregate carries
// only its WsID).
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
	// The root message's real id is surfaced so the client can edit the root by
	// the same /messages/:messageId route that addresses replies.
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

// TestThreadDTOFrom_NoMessages asserts a thread with no messages yields empty
// root fields and a non-nil empty Replies slice so the envelope serialises [].
func TestThreadDTOFrom_NoMessages(t *testing.T) {
	rt := domain.ReviewThread{ID: "t2", WsID: "w1", FilePath: "x.go", Side: domain.ReviewSideLeft}

	got := dto.ThreadDTOFrom(rt, "p1", "r1")

	assert.Empty(t, got.Body)
	assert.Empty(t, got.Author)
	assert.NotNil(t, got.Replies)
	assert.Len(t, got.Replies, 0)
}

// TestThreadDTOFrom_Unresolved asserts an open thread maps Resolved=false.
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
	assert.Len(t, got.Replies, 0)
}

// TestThreadDTOList asserts the list converter maps each thread and returns a
// non-nil empty slice for empty input.
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
	assert.Len(t, empty, 0)
}

// TestThreadDTOFrom_CarriesAgentAttributionAtBothLevels is the wire half of Task
// D. The root message is FLATTENED onto ThreadDTO rather than appearing in
// Replies, so root attribution and reply attribution are two separate mappings —
// getting one right and the other wrong would render an agent-opened thread
// attributed on its answers and anonymous on the finding itself.
//
// The fixture is deliberately mixed: an agent root, a second agent's reply, and a
// human reply. Two different providers prove the ids are carried per MESSAGE, and
// the human reply proves an unattributed message stays unattributed rather than
// inheriting the root's.
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

// TestThreadDTOFrom_PreAttributionThreadOmitsTheFields is the degradation guard
// on the wire: a thread whose messages predate attribution — the common case
// forever — must serialise exactly as it did before, with neither key present, so
// a client cannot mistake an empty string for a provider it should resolve.
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
	// The agent flag is untouched: an old agent message is still an agent message,
	// it just has nothing to attribute it to.
	assert.True(t, got.IsAgent)
}
