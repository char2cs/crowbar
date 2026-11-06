package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

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

// A retry names the refused prompt it sends again, for the ledger to link the two.
func TestSubmitPrompt_ThreadsTheRetriedPromptOntoTheSameCall(t *testing.T) {
	ctx, rec := newTestContext(t, http.MethodPost, "/prompts",
		[]byte(`{"text":"hello","clientRequestId":"9d1a5551-8145-46a1-bf09-b99d39163341","retryOf":"refused-1"}`))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}
	uc := &fakeAgentUsecase{getChat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}}
	newChatHandlers(uc).SubmitPrompt(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, uc.promptCalls, 1)
	assert.Equal(t, "refused-1", uc.promptCalls[0].retryOf)
}

// A staged model/effort travels on the SAME request as the text — the
// composer never PATCHes .../selection on its own, only bundles the pick
// with the next send. Regression for that becoming two separate calls again.
func TestSubmitPrompt_ThreadsStagedModelAndEffortOntoTheSameCall(t *testing.T) {
	ctx, rec := newTestContext(t, http.MethodPost, "/prompts",
		[]byte(`{"text":"hello","clientRequestId":"9d1a5551-8145-46a1-bf09-b99d39163341","model":"opus","effort":"high"}`))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}
	uc := &fakeAgentUsecase{
		getChat:      domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"},
		promptResult: domain.AgentPromptSubmission{RunnerID: "runner-new", TerminalSessionID: "term-new"},
	}
	newChatHandlers(uc).SubmitPrompt(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, uc.promptCalls, 1)
	assert.Equal(t, &domain.ChatSelection{Model: "opus", Effort: "high"}, uc.promptCalls[0].selection)
}

// A staged provider travels on the SAME request as the text too — the
// picker never calls a separate switch endpoint any more, only bundles the
// pick with the next send, exactly like model/effort above.
func TestSubmitPrompt_ThreadsStagedProviderOntoTheSameCall(t *testing.T) {
	ctx, rec := newTestContext(t, http.MethodPost, "/prompts",
		[]byte(`{"text":"hello","clientRequestId":"9d1a5551-8145-46a1-bf09-b99d39163341","provider":"codex"}`))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}
	uc := &fakeAgentUsecase{
		getChat:      domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"},
		promptResult: domain.AgentPromptSubmission{RunnerID: "runner-new", TerminalSessionID: "term-new"},
	}
	newChatHandlers(uc).SubmitPrompt(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, uc.promptCalls, 1)
	assert.Equal(t, "codex", uc.promptCalls[0].provider)
	assert.Nil(t, uc.promptCalls[0].selection, "a provider-only body stages no model/effort")
}

// No model/effort in the request body means the composer had nothing staged
// — the sticky selection must be left exactly as-is, not overwritten to "".
func TestSubmitPrompt_OmittedModelAndEffortMeansNothingStaged(t *testing.T) {
	ctx, rec := newTestContext(t, http.MethodPost, "/prompts",
		[]byte(`{"text":"hello","clientRequestId":"9d1a5551-8145-46a1-bf09-b99d39163341"}`))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}
	uc := &fakeAgentUsecase{
		getChat:      domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"},
		promptResult: domain.AgentPromptSubmission{RunnerID: "runner-new", TerminalSessionID: "term-new"},
	}
	newChatHandlers(uc).SubmitPrompt(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, uc.promptCalls, 1)
	assert.Nil(t, uc.promptCalls[0].selection)
}

// THE OTHER HALF of the test above, and the one "" alone could never express:
// a body that CARRIES model/effort as "" staged the provider's own default —
// a real pick the user made in the picker — and must reach the usecase as a
// present, all-empty selection. Read as "absent, keep the old one" (which a
// plain string body cannot tell apart) the chat silently keeps running the
// model it was already on and nothing can ever clear it, since the picker no
// longer PATCHes .../selection on its own.
func TestRegression_SubmitPrompt_AnExplicitlyEmptySelectionIsStagedNotIgnored(t *testing.T) {
	ctx, rec := newTestContext(t, http.MethodPost, "/prompts",
		[]byte(`{"text":"hello","clientRequestId":"9d1a5551-8145-46a1-bf09-b99d39163341","model":"","effort":""}`))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}
	uc := &fakeAgentUsecase{
		getChat:      domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"},
		promptResult: domain.AgentPromptSubmission{RunnerID: "runner-new", TerminalSessionID: "term-new"},
	}
	newChatHandlers(uc).SubmitPrompt(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, uc.promptCalls, 1)
	assert.Equal(t, &domain.ChatSelection{}, uc.promptCalls[0].selection)
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
	assert.Equal(t, problem.RequestOutcomeUncertain, envelope.Code)
}

func TestSubmitPrompt_DeniesACrossWorkspaceChat(t *testing.T) {
	ctx, rec := newTestContext(t, http.MethodPost, "/presentation", []byte(`{"text":"x","clientRequestId":"9d1a5551-8145-46a1-bf09-b99d39163341"}`))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}
	uc := &fakeAgentUsecase{getChat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-2"}}
	newChatHandlers(uc).SubmitPrompt(ctx)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, uc.promptCalls)
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
		code   problem.Code
	}{
		{agentusecase.ErrSlashCatalogUnsupported, http.StatusUnprocessableEntity, problem.CatalogUnsupported},
		{agentusecase.ErrSlashCatalogUnavailable, http.StatusFailedDependency, problem.CatalogCommandUnavailable},
		{agentusecase.ErrSlashCatalogTimeout, http.StatusGatewayTimeout, problem.CatalogTimeout},
		{agentusecase.ErrSlashCatalogMalformed, http.StatusBadGateway, problem.CatalogMalformedOutput},
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
