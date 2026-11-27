//go:build integration

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

const telemetryStubProviderDescriptorYAML = `id: telemetrystub
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  user_prompt:
    in: user_prompt
    map:
      message: prompt
  turn_stop:
    in: turn_stop
    map:
      session_id: session_id
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
telemetry:
  callback:
    format: json
    fields:
      context.used_percent:    context.usedPercent
      context.used_tokens:     context.usedTokens
      context.capacity_tokens: context.capacityTokens
`

type telemetryWire struct {
	Context *struct {
		UsedPercent *float64 `json:"usedPercent"`
	} `json:"context"`
}

func getTelemetry(t *testing.T, h *harness, imported importedRepo, chatID string) telemetryWire {
	t.Helper()
	var out telemetryWire
	h.get(repoBase(imported)+"/chats/"+chatID+"/telemetry", &out)
	return out
}

func TestRegression_TelemetrySurvivesAGracefulDaemonRestart(t *testing.T) {
	home := t.TempDir()
	h1 := newHarnessAt(t, home)
	writeProviderDescriptor(t, h1, "telemetrystub", telemetryStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h1)
	chatID, runnerID := createStubChat(t, h1, imported, "telemetrystub")

	postHook(t, h1, repoBase(imported), "telemetrystub", runnerID, "telemetry",
		`{"context":{"usedPercent":42.5,"usedTokens":1000,"capacityTokens":10000}}`)

	before := getTelemetry(t, h1, imported, chatID)
	require.NotNil(t, before.Context, "precondition: the chat has a usage report before the restart")
	require.NotNil(t, before.Context.UsedPercent)
	assert.InDelta(t, 42.5, *before.Context.UsedPercent, 0.001)

	h1.shutdown()
	h2 := newHarnessAt(t, home)

	after := getTelemetry(t, h2, imported, chatID)
	require.NotNil(t, after.Context,
		"the chat's last usage report must survive a daemon restart — this is the live bug: the compact "+
			"button and context gauge went blank after every restart because the report lived only in a "+
			"process-local map")
	require.NotNil(t, after.Context.UsedPercent)
	assert.InDelta(t, 42.5, *after.Context.UsedPercent, 0.001,
		"the recovered report must be the SAME one reported before the restart, not a reinvented zero")
}

func TestRegression_TelemetryForgetPurgesTheDurableRowAcrossARestart(t *testing.T) {
	home := t.TempDir()
	h1 := newHarnessAt(t, home)
	writeProviderDescriptor(t, h1, "telemetrystub", telemetryStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h1)

	feed := recordSidebar(t, h1)
	chatID, runnerID := createStubChat(t, h1, imported, "telemetrystub")
	feed.awaitListed(chatID)

	postHook(t, h1, repoBase(imported), "telemetrystub", runnerID, "telemetry",
		`{"context":{"usedPercent":10,"usedTokens":100,"capacityTokens":10000}}`)
	before := getTelemetry(t, h1, imported, chatID)
	require.NotNil(t, before.Context, "precondition: the chat has a usage report")

	require.NoError(t, h1.raw(http.MethodDelete, repoBase(imported)+"/chats/"+chatID, nil, http.StatusOK).Body.Close())
	feed.awaitGone(stream.KindChatEntity, chatID)
	h1.Quiesce()

	h1.shutdown()
	h2 := newHarnessAt(t, home)

	row, err := h2.app.GORM.AgentChatTelemetry.FindByKey(context.Background(), chatID)
	require.NoError(t, err)
	assert.Nil(t, row, "Forget must purge the durable row, not just the in-memory copy — a deleted chat's "+
		"last usage number must not reappear on a later restart")
}
