package domain_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestReviewThread_NormalizedMessages_EmptyMarshalsAsArray(t *testing.T) {
	thread := domain.ReviewThread{ID: "t1"}.NormalizedMessages()

	raw, err := json.Marshal(thread)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"messages":[]`)
	assert.False(t, strings.Contains(string(raw), `"messages":null`), "must never serialize null messages")
}

func TestReviewThread_NormalizedMessages_PreservesExisting(t *testing.T) {
	thread := domain.ReviewThread{
		ID:       "t1",
		Messages: []domain.ReviewMessage{{ID: "m1", Body: "hi"}},
	}.NormalizedMessages()

	require.Len(t, thread.Messages, 1)
	assert.Equal(t, "m1", thread.Messages[0].ID)
}
