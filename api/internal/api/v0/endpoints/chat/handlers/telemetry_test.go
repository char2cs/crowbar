package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
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

// A chat on a surface whose channel carries no usage report keeps no gauge:
// the store is durable, so the last number earned elsewhere would never move.
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
