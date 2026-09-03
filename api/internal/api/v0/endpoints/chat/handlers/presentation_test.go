package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestMessages_MapsBoundedLedgerPage(t *testing.T) {
	ctx, rec := newTestContext(t, http.MethodGet, "/messages?after=7&limit=20", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}
	at := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	uc := &fakeAgentUsecase{
		getChat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"},
		messagePage: domain.LedgerPage{Cursor: 8, OldestCursor: 8, Items: []domain.LedgerMessage{{
			Sequence:   8,
			LedgerTurn: domain.LedgerTurn{Role: "assistant", Provider: "codex", RunnerID: "private", Text: "done", At: at},
		}}},
	}
	newChatHandlers(uc).Messages(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []messageCall{{chatID: "chat-1", after: 7, limit: 20}}, uc.messageCalls)
	var envelope struct {
		Data dto.AgentMessagePageDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data.Items, 1)
	assert.Equal(t, "codex", envelope.Data.Items[0].ProviderID)
	assert.NotContains(t, rec.Body.String(), "private", "runner correlation metadata is never exposed")
}

func TestSubmitPrompt_ReturnsReplacementIdentity(t *testing.T) {
	ctx, rec := newTestContext(t, http.MethodPost, "/prompts", []byte(`{"text":"hello","clientRequestId":"9d1a5551-8145-46a1-bf09-b99d39163341"}`))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}
	uc := &fakeAgentUsecase{
		getChat:      domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"},
		promptResult: domain.AgentPromptSubmission{RunnerID: "runner-new", TerminalSessionID: "term-new"},
	}
	newChatHandlers(uc).SubmitPrompt(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, uc.promptCalls, 1)
	assert.Equal(t, "hello", uc.promptCalls[0].text)
	assert.Contains(t, rec.Body.String(), `"runnerId":"runner-new"`)
}

func TestSubmitPrompt_ConflictCarriesStableMachineCode(t *testing.T) {
	ctx, rec := newTestContext(t, http.MethodPost, "/prompts", []byte(`{"text":"hello","clientRequestId":"9d1a5551-8145-46a1-bf09-b99d39163341"}`))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}
	uc := &fakeAgentUsecase{
		getChat:   domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"},
		promptErr: agentusecase.ErrPromptOutcomeUnknown,
	}
	newChatHandlers(uc).SubmitPrompt(ctx)

	require.Equal(t, http.StatusConflict, rec.Code)
	var envelope libs.Envelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	assert.Equal(t, agentusecase.PromptCodeOutcomeUnknown, envelope.Code)
}

func TestMessagesAndPrompt_DenyCrossWorkspaceChat(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		ctx, rec := newTestContext(t, method, "/presentation", []byte(`{"text":"x","clientRequestId":"9d1a5551-8145-46a1-bf09-b99d39163341"}`))
		ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}
		uc := &fakeAgentUsecase{getChat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-2"}}
		if method == http.MethodGet {
			newChatHandlers(uc).Messages(ctx)
		} else {
			newChatHandlers(uc).SubmitPrompt(ctx)
		}
		assert.Equal(t, http.StatusNotFound, rec.Code)
		assert.Empty(t, uc.messageCalls)
		assert.Empty(t, uc.promptCalls)
	}
}

func TestSlashCatalog_MapsProviderNeutralEphemeralResult(t *testing.T) {
	ctx, rec := newTestContext(t, http.MethodGet, "/slash-catalog", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}
	uc := &fakeAgentUsecase{
		getChat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"},
		catalog: engineagents.SlashCatalog{
			ProviderID:   "codex",
			Completeness: engineagents.CatalogCompletenessModelVisible,
			Items: []engineagents.SlashCatalogItem{{
				ID: "skill-1", Kind: "skill", Label: "review", InsertText: "$review ", Source: "model-visible",
			}},
			Warnings: []string{"partial"},
		},
	}
	newChatHandlers(uc).SlashCatalog(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"chat-1"}, uc.catalogCalls)
	assert.Contains(t, rec.Body.String(), `"completeness":"model_visible"`)
	assert.Contains(t, rec.Body.String(), `"insertText":"$review "`)
}

func TestSlashCatalog_ErrorsCarryStableCodeAndStatus(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{agentusecase.ErrSlashCatalogUnsupported, http.StatusUnprocessableEntity, agentusecase.CatalogCodeUnsupported},
		{agentusecase.ErrSlashCatalogUnavailable, http.StatusFailedDependency, agentusecase.CatalogCodeUnavailable},
		{agentusecase.ErrSlashCatalogTimeout, http.StatusGatewayTimeout, agentusecase.CatalogCodeTimeout},
		{agentusecase.ErrSlashCatalogMalformed, http.StatusBadGateway, agentusecase.CatalogCodeMalformed},
	}
	for _, tc := range cases {
		ctx, rec := newTestContext(t, http.MethodGet, "/slash-catalog", nil)
		ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}
		uc := &fakeAgentUsecase{
			getChat:    domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"},
			catalogErr: tc.err,
		}
		newChatHandlers(uc).SlashCatalog(ctx)
		assert.Equal(t, tc.status, rec.Code)
		var envelope libs.Envelope
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
		assert.Equal(t, tc.code, envelope.Code)
	}
}
