package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestPendingPrompt_ReturnsTheRecoveredSubmission(t *testing.T) {
	uc := &fakeAgentUsecase{
		pendingPromptFound: true,
		pendingPrompt: domain.PendingPrompt{
			Text:      "please rename this function",
			State:     "dispatching",
			RequestID: "5c1b1c8a-2f3e-4a9b-9d1e-6a2b3c4d5e6f",
		},
	}
	ctx, rec := scoped(t, "/pending-prompt")
	newChatHandlers(inWorkspace(uc)).PendingPrompt(ctx)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Data dto.PendingPromptDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "please rename this function", body.Data.Text)
	assert.Equal(t, "dispatching", body.Data.State)
	assert.Equal(t, "5c1b1c8a-2f3e-4a9b-9d1e-6a2b3c4d5e6f", body.Data.RequestID)
}

func TestPendingPrompt_NothingPendingIsNoContent(t *testing.T) {
	ctx, rec := scoped(t, "/pending-prompt")
	newChatHandlers(inWorkspace(&fakeAgentUsecase{})).PendingPrompt(ctx)

	assert.Equal(t, http.StatusNoContent, ctx.Writer.Status())
	assert.Empty(t, rec.Body.String())
}

func TestPendingPrompt_RefusesAChatOutsideTheRouteScope(t *testing.T) {
	uc := &fakeAgentUsecase{
		getChat:            domain.Chat{ID: "chat-1", WorkspaceID: "another-ws"},
		pendingPromptFound: true,
	}
	ctx, rec := scoped(t, "/pending-prompt")

	newChatHandlers(uc).PendingPrompt(ctx)

	assert.NotEqual(t, http.StatusOK, rec.Code)
}
