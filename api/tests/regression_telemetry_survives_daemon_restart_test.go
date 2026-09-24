//go:build integration

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// telemetryStubProviderDescriptorYAML declares a v2-style top-level
// `telemetry.callback` block (the shape claude.yaml itself uses) so a
// `POST .../chats/hooks` with event "telemetry" actually maps to a usage
// report instead of ErrUnsupported.
const telemetryStubProviderDescriptorYAML = `id: telemetrystub
spawn:
  cmd: "cat"
  interactive_required: true
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

// getTelemetry reads a chat's context/usage report off the same endpoint the
// frontend's context gauge polls. Asserts 200 — a caller expecting 204 (no
// report) uses h.raw directly instead.
func getTelemetry(t *testing.T, h *harness, imported importedRepo, chatID string) telemetryWire {
	t.Helper()
	var out telemetryWire
	h.get(repoBase(imported)+"/chats/"+chatID+"/telemetry", &out)
	return out
}

// TestRegression_TelemetrySurvivesAGracefulDaemonRestart is the live-bug guard
// for "the compact button and context gauge disappear after every app
// restart, nightly auto-update, or crash".
//
// Root cause: telemetry.Store (internal/app/usecases/chat/internal/shared/
// telemetry) held every chat's last usage report in a plain, process-local Go
// map with no durable backing at all — telemetry.New() built an empty one on
// every daemon boot, restart or not, so a chat that had reported before the
// restart came back with NO report until its provider happened to report
// again: GET .../chats/:id/telemetry went from 200 to 204 across a restart
// even though nothing about the CHAT changed.
//
// Fixed by giving Store an optional durable backing (NewDurable, wired
// through a new domain.AgentChatTelemetry sqlite row per chat): Set/Forget now
// write through, and construction hydrates from whatever the DB already
// holds. This test drives the real ingestion path (an HTTP hook, exactly as a
// vendor CLI delivers one) and a REAL daemon teardown + rebuild over the same
// crowbar home (harness.shutdown then newHarnessAt) — not a white-box call —
// so it fails exactly the way the live bug did: 200 before, 204 after.
func TestRegression_TelemetrySurvivesAGracefulDaemonRestart(t *testing.T) {
	home := t.TempDir()
	h1 := newHarnessAt(t, home)
	writeProviderDescriptor(t, h1, "telemetrystub", telemetryStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h1)
	chatID, runnerID := createStubChat(t, h1, imported, "telemetrystub")

	postProviderHook(t, h1, imported, "telemetrystub", runnerID, "telemetry",
		`{"context":{"usedPercent":42.5,"usedTokens":1000,"capacityTokens":10000}}`)

	before := getTelemetry(t, h1, imported, chatID)
	require.NotNil(t, before.Context, "precondition: the chat has a usage report before the restart")
	require.NotNil(t, before.Context.UsedPercent)
	assert.InDelta(t, 42.5, *before.Context.UsedPercent, 0.001)

	// A graceful stop + a fresh boot over the SAME home — an app relaunch or a
	// nightly auto-update, not a crash. Real container teardown and rebuild,
	// not a simulated one.
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

// TestRegression_TelemetryForgetPurgesTheDurableRowAcrossARestart pins the
// other half of durability: Forget (chat deletion) must purge the DURABLE row
// too, not just the in-memory copy — otherwise a deleted chat's last usage
// number would silently reappear (under a reused id, or as a leaked row no
// UI ever reads but that never gets cleaned up) on every future restart.
//
// Asserted directly against the persisted store (h2.app.GORM.AgentChatTelemetry)
// rather than through the chat-scoped HTTP route: the chat itself is gone
// after delete, so that route 404s regardless of whether the telemetry row was
// actually purged — checking the row directly is the only way to pin Forget's
// own durable-delete behaviour rather than a fact implied by something else.
func TestRegression_TelemetryForgetPurgesTheDurableRowAcrossARestart(t *testing.T) {
	home := t.TempDir()
	h1 := newHarnessAt(t, home)
	writeProviderDescriptor(t, h1, "telemetrystub", telemetryStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h1)

	frames := dialAgentWS(t, h1, repoBase(imported)+"/chats/ws")
	chatID, runnerID := createStubChat(t, h1, imported, "telemetrystub")
	waitForChatFrame(t, frames, chatID, "created")

	postProviderHook(t, h1, imported, "telemetrystub", runnerID, "telemetry",
		`{"context":{"usedPercent":10,"usedTokens":100,"capacityTokens":10000}}`)
	before := getTelemetry(t, h1, imported, chatID)
	require.NotNil(t, before.Context, "precondition: the chat has a usage report")

	h1.raw(http.MethodDelete, repoBase(imported)+"/chats/"+chatID, nil, http.StatusAccepted).Body.Close()
	waitForChatFrame(t, frames, chatID, "deleted")
	h1.Quiesce()

	h1.shutdown()
	h2 := newHarnessAt(t, home)

	row, err := h2.app.GORM.AgentChatTelemetry.FindByKey(context.Background(), chatID)
	require.NoError(t, err)
	assert.Nil(t, row, "Forget must purge the durable row, not just the in-memory copy — a deleted chat's "+
		"last usage number must not reappear on a later restart")
}
