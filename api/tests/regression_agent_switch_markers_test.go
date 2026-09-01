//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// switchableStubProviderDescriptorYAML is streamStubProviderDescriptorYAML
// (see regression_agent_message_stream_test.go) plus a declared model/effort
// catalogue: SetChatSelection's validateSelection refuses a value outside
// the provider's own available list, and streamstub/quietstub declare none —
// only the selection-marker tests below need one.
const switchableStubProviderDescriptorYAML = `id: switchablestub
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
model:
  available: [sonnet, opus]
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--model", value: "{model}" }
effort:
  available:
    "*": [low, high]
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--effort", value: "{effort}" }
runtime:
  transport: hooks
  hooks:
    format: json
`

type interruptionDTO struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

func readInterruptions(t *testing.T, h *harness, imported importedRepo, chatID string) []interruptionDTO {
	t.Helper()
	var activity struct {
		Interruptions []interruptionDTO `json:"interruptions"`
	}
	h.get(wsBase(imported)+"/chats/"+chatID+"/activity", &activity)
	return activity.Interruptions
}

// TestRegression_ProviderSwitchRecordsAMarkerInterruption is the integration
// counterpart of the internal/runner package's
// TestRegression_CloseAbandonedTurn_* family and chat_test.go's
// TestSwitchProvider_RecordsAProviderSwitchedInterruption — those exercise
// switchProviderLocked directly; this drives the real POST .../switch
// endpoint the composer actually calls, and reads the marker back through
// the real GET .../activity endpoint the transcript actually renders from.
func TestRegression_ProviderSwitchRecordsAMarkerInterruption(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, _ := createStubChat(t, h, imported, "streamstub")

	var result struct {
		ID string `json:"id"`
	}
	h.post(wsBase(imported)+"/chats/"+chatID+"/switch",
		map[string]string{"provider": "quietstub"}, http.StatusOK, &result)
	h.Quiesce()

	interruptions := readInterruptions(t, h, imported, chatID)
	require.Len(t, interruptions, 1)
	assert.Equal(t, "provider_switched", interruptions[0].Kind)
	assert.Equal(t, "quietstub", interruptions[0].Detail,
		"the marker must name the TARGET provider, for the divider's own label")
}

// TestRegression_SwitchingToTheSameProviderRecordsNoMarker proves the no-op
// guard through the real HTTP path: resuming into the provider a chat is
// already on is not a switch, and must not draw a divider that says nothing
// actually changed.
func TestRegression_SwitchingToTheSameProviderRecordsNoMarker(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, _ := createStubChat(t, h, imported, "streamstub")

	var result struct {
		ID string `json:"id"`
	}
	h.post(wsBase(imported)+"/chats/"+chatID+"/switch",
		map[string]string{"provider": "streamstub"}, http.StatusOK, &result)
	h.Quiesce()

	assert.Empty(t, readInterruptions(t, h, imported, chatID))
}

// TestRegression_SelectionChangeRecordsModelAndEffortMarkers is the
// integration counterpart of chat_test.go's SetChatSelection unit coverage:
// drives the real PATCH .../selection endpoint the model/effort pickers
// actually call, and proves BOTH halves of one call get their own marker —
// a single SetChatSelection call can change model and effort together, and
// the transcript needs a pill for each.
func TestRegression_SelectionChangeRecordsModelAndEffortMarkers(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "switchablestub", switchableStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, _ := createStubChat(t, h, imported, "switchablestub")

	resp := h.raw(http.MethodPatch, wsBase(imported)+"/chats/"+chatID+"/selection",
		map[string]string{"model": "opus", "effort": "high"}, http.StatusAccepted)
	_ = resp.Body.Close()
	h.Quiesce()

	interruptions := readInterruptions(t, h, imported, chatID)
	require.Len(t, interruptions, 2)
	kinds := []string{interruptions[0].Kind, interruptions[1].Kind}
	details := []string{interruptions[0].Detail, interruptions[1].Detail}
	assert.ElementsMatch(t, []string{"model_changed", "effort_changed"}, kinds)
	assert.ElementsMatch(t, []string{"opus", "high"}, details)
}

// TestRegression_SelectionChangeOfOnlyOneHalfRecordsOneMarker proves the
// halves are independent: changing just the effort (model left as-is) must
// not ALSO draw a model-changed pill for a value that never actually moved.
func TestRegression_SelectionChangeOfOnlyOneHalfRecordsOneMarker(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "switchablestub", switchableStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, _ := createStubChat(t, h, imported, "switchablestub")

	resp := h.raw(http.MethodPatch, wsBase(imported)+"/chats/"+chatID+"/selection",
		map[string]string{"model": "", "effort": "low"}, http.StatusAccepted)
	_ = resp.Body.Close()
	h.Quiesce()

	interruptions := readInterruptions(t, h, imported, chatID)
	require.Len(t, interruptions, 1)
	assert.Equal(t, "effort_changed", interruptions[0].Kind)
	assert.Equal(t, "low", interruptions[0].Detail)
}
