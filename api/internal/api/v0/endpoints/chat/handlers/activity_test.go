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
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func scoped(t *testing.T, target string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	ctx, rec := newTestContext(t, http.MethodGet, target, nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}
	return ctx, rec
}

func inWorkspace(uc *fakeAgentUsecase) *fakeAgentUsecase {
	uc.getChat = domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}
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

// A subagent's own nested tool call and its own reply history must reach the
// wire — the durable nested transcript this DTO exists to carry, not just
// the flat start/end marker the shape used to be limited to.
func TestActivity_CarriesASubagentsOwnNestedToolCallAndMessages(t *testing.T) {
	ended := activityAt.Add(time.Second)
	uc := &fakeAgentUsecase{activity: agentusecase.ChatActivity{
		ToolCalls: []domain.ActivityToolCall{{
			ID: "child-tool-1", Seq: 1, Name: "Bash", SubagentID: "thread-child",
			Status: domain.ToolStatusOK, StartedAt: activityAt, EndedAt: &ended,
		}},
		Subagents: []domain.ActivitySubagent{{
			ID: "thread-child", Seq: 2, StartedAt: activityAt, EndedAt: &ended,
			Messages: []domain.ActivitySubagentMessage{{Text: "done", At: ended}},
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
	assert.Equal(t, "thread-child", body.Data.ToolCalls[0].SubagentID)
	assert.Empty(t, body.Data.ToolCalls[0].TurnID, "a nested tool call has no top-level turn")

	require.Len(t, body.Data.Subagents, 1)
	require.Len(t, body.Data.Subagents[0].Messages, 1)
	assert.Equal(t, "done", body.Data.Subagents[0].Messages[0].Text)
}

// An ordinary top-level tool call and a subagent with no reply yet must not
// grow a subagentId or a messages array out of nothing.
func TestActivity_OrdinaryToolCallsAndFreshSubagentsCarryNoNestedFields(t *testing.T) {
	uc := &fakeAgentUsecase{activity: agentusecase.ChatActivity{
		ToolCalls: []domain.ActivityToolCall{{
			ID: "tool-1", TurnID: "turn-1", Seq: 1, Name: "Edit",
			Status: domain.ToolStatusOK, StartedAt: activityAt,
		}},
		Subagents: []domain.ActivitySubagent{{
			ID: "a1", TurnID: "turn-1", Seq: 2, StartedAt: activityAt,
		}},
	}}
	ctx, rec := scoped(t, "/activity")
	newChatHandlers(inWorkspace(uc)).Activity(ctx)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), `"subagentId"`)
	assert.NotContains(t, rec.Body.String(), `"messages"`)
}

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

func TestTelemetry_NoReportIsNoContent(t *testing.T) {
	ctx, rec := scoped(t, "/telemetry")
	newChatHandlers(inWorkspace(&fakeAgentUsecase{})).Telemetry(ctx)

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
	uc := &fakeAgentUsecase{getChat: domain.Chat{ID: "chat-1", WorkspaceID: "another-ws"}}
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
	uc := &fakeAgentUsecase{getChat: domain.Chat{ID: "chat-1", WorkspaceID: "another-ws"}}
	ctx, rec := scoped(t, "/activity/tool-1/payload?side=request")
	ctx.Params = append(ctx.Params, gin.Param{Key: "toolId", Value: "tool-1"})

	newChatHandlers(uc).ToolPayload(ctx)

	assert.NotEqual(t, http.StatusOK, rec.Code)
}

func TestTelemetry_RefusesAChatOutsideTheRouteScope(t *testing.T) {
	uc := &fakeAgentUsecase{
		getChat:     domain.Chat{ID: "chat-1", WorkspaceID: "another-ws"},
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

			{
				ID:    "suggestion-0",
				Kind:  domain.ChoiceOptionSuggestion,
				Label: "Switch to a more permissive mode",
			},
		},
		At: activityAt,
	}
}

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

func TestChoices_CarriesEveryQuestionOfAMultiQuestionPrompt(t *testing.T) {
	asked := domain.ActivityChoice{
		ID: "choice-2", TurnID: "turn-1", Seq: 7,
		Kind: domain.ChoiceKindQuestion, ToolName: "AskUserQuestion", At: activityAt,
		Questions: []domain.ActivityChoiceQuestion{
			{ID: "q0", Text: "Which language?", Options: []domain.ActivityChoiceOption{
				{ID: "q0-answer-0", Kind: domain.ChoiceOptionAnswer, Label: "Go"},
			}},
			{
				ID: "q1", Text: "Which databases?", Multi: true,
				Options: []domain.ActivityChoiceOption{
					{ID: "q1-answer-0", Kind: domain.ChoiceOptionAnswer, Label: "SQLite"},
				},
			},
		},
	}
	uc := &fakeAgentUsecase{pending: []domain.ActivityChoice{asked}}
	ctx, rec := scoped(t, "/choices")
	newChatHandlers(inWorkspace(uc)).Choices(ctx)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Data []dto.AgentChoiceDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	require.Len(t, body.Data[0].Questions, 2)
	assert.Equal(t, "Which language?", body.Data[0].Questions[0].Text)
	assert.Equal(t, "q0-answer-0", body.Data[0].Questions[0].Options[0].ID)
	assert.True(t, body.Data[0].Questions[1].Multi, "multiSelect is per question")
}

func TestChoices_OmitsTheQuestionListForAPromptThatAsksNone(t *testing.T) {
	uc := &fakeAgentUsecase{pending: []domain.ActivityChoice{pendingChoice()}}
	ctx, rec := scoped(t, "/choices")
	newChatHandlers(inWorkspace(uc)).Choices(ctx)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), `"questions"`)
}

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

func TestChoices_RefusesAChatOutsideTheScopedWorkspace(t *testing.T) {
	uc := &fakeAgentUsecase{getChat: domain.Chat{ID: "chat-1", WorkspaceID: "other"}}
	ctx, rec := scoped(t, "/choices")
	newChatHandlers(uc).Choices(ctx)

	assert.NotEqual(t, http.StatusOK, rec.Code)
	assert.Empty(t, uc.pendingCalls)
}

// TestRegression_TelemetryIsAbsentOnASurfaceThatCarriesNone is the measured
// half of the capability fix. codex publishes usage only as
// thread/tokenUsage/updated, an app-server notification: NO recorded codex
// hooks payload carries a token, usage, context or cost field
// (TestRegression_CodexReportsNoUsageOnItsHooksChannel, engine/agents, lists
// the three captures and the eleven hooks it wires). So a codex chat on its
// hooks-channel terminal surface can never receive another report.
//
// The store is DURABLE, which is what made this visible: a chat that earned a
// report on the chat surface and then switched kept serving that number
// forever — a gauge that never moves. Absence, never a dead control (the
// house rule context-gauge.tsx states for itself).
func TestRegression_TelemetryIsAbsentOnASurfaceThatCarriesNone(t *testing.T) {
	capacity, used := 200000, 37117
	pct := 19.0
	uc := &fakeAgentUsecase{
		telemetryOK: true,
		telemetry: engineagents.Telemetry{
			ObservedAt: activityAt,
			Source:     engineagents.TelemetrySourceCallback,
			Context:    &engineagents.ContextUsage{CapacityTokens: &capacity, UsedTokens: &used, UsedPercent: &pct},
		},
		telemetryOffSurface: true,
	}
	ctx, rec := scoped(t, "/telemetry")
	newChatHandlers(inWorkspace(uc)).Telemetry(ctx)

	assert.Equal(t, http.StatusNoContent, ctx.Writer.Status())
	assert.Empty(t, rec.Body.String(), "a stale number is worse than no gauge at all")
}
