package handlers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/agent"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// scoped builds a gin context already carrying the route scope the handlers
// require, matching how the rest of this package drives them.
func scoped(t *testing.T, target string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	ctx, rec := newTestContext(t, http.MethodGet, target, nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}
	return ctx, rec
}

func inWorkspace(uc *fakeAgentUsecase) *fakeAgentUsecase {
	uc.getChat = domain.AgentChat{ID: "chat-1", WorkspaceID: "ws-1"}
	return uc
}

var activityAt = time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

func TestActivity_ReturnsWhatTheAgentDid(t *testing.T) {
	ended := activityAt.Add(time.Second)
	uc := &fakeAgentUsecase{activity: agentusecase.ChatActivity{
		ToolCalls: []domain.ActivityToolCall{{
			ID: "tool-1", TurnID: "turn-1", Seq: 3, Name: "Edit", Target: "a.go",
			RequestRef: "sha256:abc", ResultRef: "sha256:def",
			Status: domain.ToolStatusOK, DurationMS: 12, StartedAt: activityAt, EndedAt: &ended,
		}},
		Subagents: []domain.ActivitySubagent{{
			ID: "a1", TurnID: "turn-1", Seq: 4, AgentType: "explore", StartedAt: activityAt,
		}},
		Interruptions: []domain.ActivityInterruption{{
			ID: "i1", TurnID: "turn-1", Seq: 5, Kind: "permission", Detail: "Bash", At: activityAt,
		}},
	}}
	ctx, rec := scoped(t, "/activity")
	newChatHandlers(inWorkspace(uc)).Activity(ctx)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Data dto.AgentActivityDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data.ToolCalls, 1)
	assert.Equal(t, "Edit", body.Data.ToolCalls[0].Name)
	assert.Equal(t, "a.go", body.Data.ToolCalls[0].Target)
	assert.True(t, body.Data.ToolCalls[0].HasRequest)
	assert.True(t, body.Data.ToolCalls[0].HasResult)
	require.Len(t, body.Data.Subagents, 1)
	assert.Equal(t, "explore", body.Data.Subagents[0].AgentType)
	require.Len(t, body.Data.Interruptions, 1)
	assert.Equal(t, "permission", body.Data.Interruptions[0].Kind)
}

// A content ref is a global address. Publishing it would let a client ask for any
// chat's payload, so the wire carries only whether one exists.
func TestActivity_NeverPublishesAContentRef(t *testing.T) {
	uc := &fakeAgentUsecase{activity: agentusecase.ChatActivity{
		ToolCalls: []domain.ActivityToolCall{{
			ID: "tool-1", RequestRef: "sha256:secretaddress", Status: domain.ToolStatusOK,
		}},
	}}
	ctx, rec := scoped(t, "/activity")
	newChatHandlers(inWorkspace(uc)).Activity(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "sha256:secretaddress")
}

func TestActivity_EmptyListsRenderAsEmptyNotNull(t *testing.T) {
	ctx, rec := scoped(t, "/activity")
	newChatHandlers(inWorkspace(&fakeAgentUsecase{})).Activity(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"toolCalls":[]`)
	assert.Contains(t, rec.Body.String(), `"subagents":[]`)
	assert.Contains(t, rec.Body.String(), `"interruptions":[]`)
}

func TestActivity_PassesThePagingCursorThrough(t *testing.T) {
	uc := &fakeAgentUsecase{}
	ctx, rec := scoped(t, "/activity?after=7&limit=20")
	newChatHandlers(inWorkspace(uc)).Activity(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, uc.activityCalls, 1)
	assert.Equal(t, int64(7), uc.activityCalls[0].after)
	assert.Equal(t, 20, uc.activityCalls[0].limit)
}

func TestToolPayload_ServesTheRawBytes(t *testing.T) {
	uc := &fakeAgentUsecase{payload: []byte("the exact tool output\n")}
	ctx, rec := scoped(t, "/activity/tool-1/payload?side=result")
	ctx.Params = append(ctx.Params, gin.Param{Key: "toolId", Value: "tool-1"})
	newChatHandlers(inWorkspace(uc)).ToolPayload(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "the exact tool output\n", rec.Body.String(),
		"re-encoding a payload would corrupt the thing the user asked to see")
	require.Len(t, uc.payloadCalls, 1)
	assert.Equal(t, "result", uc.payloadCalls[0].side)
	assert.Equal(t, "tool-1", uc.payloadCalls[0].toolID)
}

func TestToolPayload_RefusesAnUnknownSide(t *testing.T) {
	for _, side := range []string{"", "both", "stdout"} {
		ctx, rec := scoped(t, "/activity/tool-1/payload?side="+side)
		ctx.Params = append(ctx.Params, gin.Param{Key: "toolId", Value: "tool-1"})
		newChatHandlers(inWorkspace(&fakeAgentUsecase{})).ToolPayload(ctx)
		assert.Equal(t, http.StatusBadRequest, rec.Code, "side %q", side)
	}
}

// Retention may legitimately have swept a payload: that is a fact about the
// payload, not a failure of the request.
func TestToolPayload_ASweptPayloadIsNotFound(t *testing.T) {
	uc := &fakeAgentUsecase{payloadErr: agentactivity.ErrNotFound}
	ctx, rec := scoped(t, "/activity/tool-1/payload?side=request")
	ctx.Params = append(ctx.Params, gin.Param{Key: "toolId", Value: "tool-1"})
	newChatHandlers(inWorkspace(uc)).ToolPayload(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestTelemetry_ReturnsTheProvidersReport(t *testing.T) {
	capacity, used := 200000, 37117
	pct := 19.0
	uc := &fakeAgentUsecase{
		telemetryOK: true,
		telemetry: engineagents.Telemetry{
			ObservedAt: activityAt,
			Source:     engineagents.TelemetrySourceCallback,
			Context: &engineagents.ContextUsage{
				CapacityTokens: &capacity, UsedTokens: &used, UsedPercent: &pct,
			},
			RateLimits: []engineagents.RateLimitWindow{{ID: "five_hour", UsedPercent: &pct}},
			Model:      &engineagents.ModelIdentity{ID: "m", DisplayName: "M"},
		},
	}
	ctx, rec := scoped(t, "/telemetry")
	newChatHandlers(inWorkspace(uc)).Telemetry(ctx)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Data dto.AgentTelemetryDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotNil(t, body.Data.Context)
	assert.Equal(t, 200000, *body.Data.Context.CapacityTokens)
	assert.Nil(t, body.Data.Context.RemainingPercent,
		"a fact the provider did not report must stay absent, not become zero")
	require.Len(t, body.Data.RateLimits, 1)
	require.NotNil(t, body.Data.Model)
	assert.Equal(t, "m", body.Data.Model.ID)
	assert.Nil(t, body.Data.Cost)
}

// A provider that has not reported is different from one reporting zero. The
// client draws no gauge rather than an empty one.
func TestTelemetry_NoReportIsNoContent(t *testing.T) {
	ctx, rec := scoped(t, "/telemetry")
	newChatHandlers(inWorkspace(&fakeAgentUsecase{})).Telemetry(ctx)

	// Read the status off the writer, not the recorder: a bodyless response is
	// flushed by the router on return, which a context-driven test never reaches.
	assert.Equal(t, http.StatusNoContent, ctx.Writer.Status())
	assert.Empty(t, rec.Body.String())
}

func TestActivity_RejectsANonNumericCursor(t *testing.T) {
	for _, q := range []string{"?after=soon", "?limit=lots"} {
		ctx, rec := scoped(t, "/activity"+q)
		newChatHandlers(inWorkspace(&fakeAgentUsecase{})).Activity(ctx)
		assert.Equal(t, http.StatusBadRequest, rec.Code, q)
	}
}

func TestActivity_SurfacesAReadFailure(t *testing.T) {
	uc := &fakeAgentUsecase{activityErr: errors.New("record unavailable")}
	ctx, rec := scoped(t, "/activity")

	newChatHandlers(inWorkspace(uc)).Activity(ctx)

	assert.GreaterOrEqual(t, rec.Code, http.StatusInternalServerError)
}

func TestActivity_RefusesAChatOutsideTheRouteScope(t *testing.T) {
	uc := &fakeAgentUsecase{getChat: domain.AgentChat{ID: "chat-1", WorkspaceID: "another-ws"}}
	ctx, rec := scoped(t, "/activity")

	newChatHandlers(uc).Activity(ctx)

	assert.NotEqual(t, http.StatusOK, rec.Code,
		"a chat id is not itself an authorisation for the workspace in the route")
}

func TestToolPayload_SurfacesAReadFailure(t *testing.T) {
	uc := &fakeAgentUsecase{payloadErr: errors.New("content store unavailable")}
	ctx, rec := scoped(t, "/activity/tool-1/payload?side=request")
	ctx.Params = append(ctx.Params, gin.Param{Key: "toolId", Value: "tool-1"})

	newChatHandlers(inWorkspace(uc)).ToolPayload(ctx)

	assert.GreaterOrEqual(t, rec.Code, http.StatusInternalServerError)
}

func TestToolPayload_RefusesAChatOutsideTheRouteScope(t *testing.T) {
	uc := &fakeAgentUsecase{getChat: domain.AgentChat{ID: "chat-1", WorkspaceID: "another-ws"}}
	ctx, rec := scoped(t, "/activity/tool-1/payload?side=request")
	ctx.Params = append(ctx.Params, gin.Param{Key: "toolId", Value: "tool-1"})

	newChatHandlers(uc).ToolPayload(ctx)

	assert.NotEqual(t, http.StatusOK, rec.Code)
}

func TestTelemetry_RefusesAChatOutsideTheRouteScope(t *testing.T) {
	uc := &fakeAgentUsecase{
		getChat:     domain.AgentChat{ID: "chat-1", WorkspaceID: "another-ws"},
		telemetryOK: true,
	}
	ctx, rec := scoped(t, "/telemetry")

	newChatHandlers(uc).Telemetry(ctx)

	assert.NotEqual(t, http.StatusOK, rec.Code)
}

func TestTelemetry_CarriesCostWhenTheProviderReportsIt(t *testing.T) {
	usd := 0.0649
	ms := 8123
	uc := &fakeAgentUsecase{
		telemetryOK: true,
		telemetry: engineagents.Telemetry{
			Source: engineagents.TelemetrySourceProbe,
			Cost:   &engineagents.SessionCost{TotalUSD: &usd, APIDurationMS: &ms},
		},
	}
	ctx, rec := scoped(t, "/telemetry")

	newChatHandlers(inWorkspace(uc)).Telemetry(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data dto.AgentTelemetryDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotNil(t, body.Data.Cost)
	assert.InDelta(t, 0.0649, *body.Data.Cost.TotalUSD, 0.00001)
	assert.Nil(t, body.Data.Context)
	assert.Equal(t, engineagents.TelemetrySourceProbe, body.Data.Source)
}

func pendingChoice() domain.ActivityChoice {
	return domain.ActivityChoice{
		ID: "choice-1", TurnID: "turn-1", Seq: 6,
		Kind: domain.ChoiceKindPermission, PromptID: "81899da5",
		ToolID: "tool-1", ToolName: "Bash", Title: "Bash",
		Options: []domain.ActivityChoiceOption{
			{ID: "allow", Kind: domain.ChoiceOptionAllow, Label: "Allow"},
			{ID: "suggestion-0", Kind: domain.ChoiceOptionSuggestion, Label: "setMode"},
		},
		At: activityAt,
	}
}

// The prompt is the one piece of agent state a user can act on, so the timeline
// has to carry it beside what the agent did on its own.
func TestActivity_CarriesThePromptsTheAgentPutToAHuman(t *testing.T) {
	uc := &fakeAgentUsecase{activity: agentusecase.ChatActivity{
		Choices: []domain.ActivityChoice{pendingChoice()},
	}}
	ctx, rec := scoped(t, "/activity")
	newChatHandlers(inWorkspace(uc)).Activity(ctx)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Data dto.AgentActivityDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data.Choices, 1)
	assert.Equal(t, domain.ChoiceKindPermission, body.Data.Choices[0].Kind)
	assert.True(t, body.Data.Choices[0].Pending)
	require.Len(t, body.Data.Choices[0].Options, 2)
	assert.Equal(t, "allow", body.Data.Choices[0].Options[0].ID)
}

// A failed tool says WHY inline, so a timeline row does not need a payload fetch
// to explain itself.
func TestActivity_AFailedCallCarriesItsErrorInline(t *testing.T) {
	uc := &fakeAgentUsecase{activity: agentusecase.ChatActivity{
		ToolCalls: []domain.ActivityToolCall{{
			ID: "tool-1", Name: "Bash", Status: domain.ToolStatusError,
			Error: "exit status 1", StartedAt: activityAt,
		}},
	}}
	ctx, rec := scoped(t, "/activity")
	newChatHandlers(inWorkspace(uc)).Activity(ctx)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Data dto.AgentActivityDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data.ToolCalls, 1)
	assert.Equal(t, "exit status 1", body.Data.ToolCalls[0].Error)
}

func TestChoices_ReturnsWhatTheAgentIsWaitingOn(t *testing.T) {
	uc := &fakeAgentUsecase{pending: []domain.ActivityChoice{pendingChoice()}}
	ctx, rec := scoped(t, "/choices")
	newChatHandlers(inWorkspace(uc)).Choices(ctx)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Data []dto.AgentChoiceDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, "choice-1", body.Data[0].ID)
	assert.Equal(t, "Bash", body.Data[0].ToolName)
	assert.Equal(t, []string{"chat-1"}, uc.pendingCalls)
}

// "Nothing pending" is an ANSWER, not a missing resource: a client that renders
// nothing for it is correct, and a 404 would read as breakage.
func TestChoices_AnAgentWaitingOnNothingReturnsAnEmptyList(t *testing.T) {
	ctx, rec := scoped(t, "/choices")
	newChatHandlers(inWorkspace(&fakeAgentUsecase{})).Choices(ctx)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"data":[]`)
}

func TestChoices_PropagatesAReadFailure(t *testing.T) {
	uc := &fakeAgentUsecase{pendingErr: errors.New("read model unavailable")}
	ctx, rec := scoped(t, "/choices")
	newChatHandlers(inWorkspace(uc)).Choices(ctx)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// A chat in another workspace is not this workspace's to read.
func TestChoices_RefusesAChatOutsideTheScopedWorkspace(t *testing.T) {
	uc := &fakeAgentUsecase{getChat: domain.AgentChat{ID: "chat-1", WorkspaceID: "other"}}
	ctx, rec := scoped(t, "/choices")
	newChatHandlers(uc).Choices(ctx)

	assert.NotEqual(t, http.StatusOK, rec.Code)
	assert.Empty(t, uc.pendingCalls)
}
