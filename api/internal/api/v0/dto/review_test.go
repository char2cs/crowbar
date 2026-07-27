package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestReviewMessageDTOFromMapsFields(
	t *testing.T,
) {
	createdAt := time.Date(2026, 6, 6, 12, 30, 0, 0, time.UTC)
	got := dto.ReviewMessageDTOFrom(domain.ReviewMessage{
		ID:        "msg-1",
		Author:    "alice",
		IsAgent:   true,
		Body:      "looks good",
		CreatedAt: createdAt,
	})

	assert.Equal(t, "msg-1", got.ID)
	assert.Equal(t, "alice", got.Author)
	assert.True(t, got.IsAgent)
	assert.Equal(t, "looks good", got.Body)
	assert.Equal(t, "2026-06-06T12:30:00Z", got.CreatedAt)
}

func TestReviewThreadDTOFromMapsFieldsAndResolved(
	t *testing.T,
) {
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name           string
		status         domain.ReviewThreadStatus
		wantIsResolved bool
	}{
		{
			name:           "open thread is unresolved",
			status:         domain.ReviewThreadStatusOpen,
			wantIsResolved: false,
		},
		{
			name:           "resolved thread is resolved",
			status:         domain.ReviewThreadStatusResolved,
			wantIsResolved: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := dto.ReviewThreadDTOFrom(domain.ReviewThread{
				ID:         "thread-1",
				WsID:       "ws-1",
				FilePath:   "main.go",
				LineNumber: 42,
				Side:       domain.ReviewSideRight,
				Status:     tc.status,
				Messages: []domain.ReviewMessage{
					{ID: "msg-1", Body: "first", CreatedAt: createdAt},
				},
				CreatedAt: createdAt,
			})

			assert.Equal(t, "thread-1", got.ID)
			assert.Equal(t, "ws-1", got.WsID)
			assert.Equal(t, "main.go", got.FilePath)
			assert.Equal(t, 42, got.LineNumber)
			assert.Equal(t, domain.ReviewSideRight, got.Side)
			assert.Equal(t, tc.status, got.Status)
			assert.Equal(t, tc.wantIsResolved, got.IsResolved)
			assert.Equal(t, "2026-01-02T03:04:05Z", got.CreatedAt)
			require.Len(t, got.Messages, 1)
			assert.Equal(t, "msg-1", got.Messages[0].ID)
		})
	}
}

func TestReviewThreadDTOFromEmptyMessagesIsNonNil(
	t *testing.T,
) {
	got := dto.ReviewThreadDTOFrom(domain.ReviewThread{
		ID: "thread-1",
	})

	require.NotNil(t, got.Messages)
	assert.Empty(t, got.Messages)
}

func TestReviewThreadDTOListEmptyIsNonNil(
	t *testing.T,
) {
	got := dto.ReviewThreadDTOList(nil)

	require.NotNil(t, got)
	assert.Empty(t, got)
}

func TestReviewThreadDTOListMapsEach(
	t *testing.T,
) {
	got := dto.ReviewThreadDTOList([]domain.ReviewThread{
		{ID: "thread-1"},
		{ID: "thread-2"},
	})

	require.Len(t, got, 2)
	assert.Equal(t, "thread-1", got[0].ID)
	assert.Equal(t, "thread-2", got[1].ID)
}

func TestBranchReviewDTOFromMapsFields(
	t *testing.T,
) {
	got := dto.BranchReviewDTOFrom(domain.BranchReview{
		Description:   "a review",
		MergeStrategy: gitdomain.MergeStrategy("squash"),
		Threads: []domain.ReviewThread{
			{ID: "thread-1"},
		},
	})

	assert.Equal(t, "a review", got.Description)
	assert.Equal(t, "squash", got.MergeStrategy)
	require.Len(t, got.Threads, 1)
	assert.Equal(t, "thread-1", got.Threads[0].ID)
}

func TestBranchReviewDTOFromEmptySlicesAreNonNil(
	t *testing.T,
) {
	got := dto.BranchReviewDTOFrom(domain.BranchReview{})

	require.NotNil(t, got.Threads)
	assert.Empty(t, got.Threads)
}

// TestBranchReviewDTOFromHasNoConversationsKey guards the Chat-aggregate removal
// (decision 2): the branch-review wire model must no longer carry a
// `conversations` key. The frontend degrades to `raw.conversations ?? []`, so
// its absence is FE-safe (review-api.ts).
func TestBranchReviewDTOFromHasNoConversationsKey(
	t *testing.T,
) {
	raw, err := json.Marshal(dto.BranchReviewDTOFrom(domain.BranchReview{}))
	require.NoError(t, err)

	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &decoded))

	_, hasConversations := decoded["conversations"]
	assert.False(t, hasConversations,
		"branch-review DTO must not carry a conversations key after Chat removal")
}
