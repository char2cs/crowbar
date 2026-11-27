package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

const legacyMessageJSON = `{
	"id":"m1",
	"author":"alice",
	"isAgent":false,
	"body":"this is the old shape",
	"createdAt":"2026-06-19T10:00:00Z"
}`

func TestReviewMessage_PreAttributionBlobRoundTrips(t *testing.T) {
	var msg domain.ReviewMessage
	require.NoError(t, json.Unmarshal([]byte(legacyMessageJSON), &msg))

	assert.Equal(t, "m1", msg.ID)
	assert.Equal(t, "alice", msg.Author)
	assert.Equal(t, "this is the old shape", msg.Body)
	assert.False(t, msg.IsAgent)
	assert.Empty(t, msg.ProviderID, "a pre-attribution message names no provider")
	assert.Empty(t, msg.ChatID, "a pre-attribution message names no chat")

	out, err := json.Marshal(msg)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "providerId")
	assert.NotContains(t, string(out), "chatId")
}

func TestReviewMessage_AttributionSerialisesWhenPresent(t *testing.T) {
	msg := domain.ReviewMessage{
		ID:         "m2",
		Author:     "claude",
		IsAgent:    true,
		ProviderID: "claude",
		ChatID:     "chat-7",
		Body:       "This leaks the token.",
		CreatedAt:  time.Date(2026, 6, 19, 10, 1, 0, 0, time.UTC),
	}

	out, err := json.Marshal(msg)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"providerId":"claude"`)
	assert.Contains(t, string(out), `"chatId":"chat-7"`)

	var back domain.ReviewMessage
	require.NoError(t, json.Unmarshal(out, &back))
	assert.Equal(t, msg, back)
}
