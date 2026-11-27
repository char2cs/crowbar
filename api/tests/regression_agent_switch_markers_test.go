//go:build integration

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

const switchableStubProviderDescriptorYAML = `id: switchablestub
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

const otherSwitchableStubProviderDescriptorYAML = `id: otherswitchablestub
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
  available: [haiku]
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--model", value: "{model}" }
effort:
  available:
    "*": [minimal, maximal]
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--effort", value: "{effort}" }
runtime:
  transport: hooks
  hooks:
    format: json
`

func readInterruptions(t *testing.T, h *harness, _ importedRepo, chatID string) []ledger.Marker {
	t.Helper()
	return rowsOf(t, chatRows(t, h, chatID), ledger.KindMarker, func(ledger.Marker) bool { return true })
}

func TestRegression_ProviderSwitchRecordsAMarkerInterruption(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, _ := createStubChat(t, h, imported, "streamstub")

	var result struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats/"+chatID+"/switch",
		map[string]string{"provider": "quietstub"}, http.StatusOK, &result)
	h.Quiesce()

	interruptions := readInterruptions(t, h, imported, chatID)
	require.Len(t, interruptions, 1)
	assert.Equal(t, "provider_switched", interruptions[0].Kind)
	assert.Equal(t, "quietstub", interruptions[0].Detail,
		"the marker must name the TARGET provider, for the divider's own label")
}

func TestRegression_SwitchingToTheSameProviderRecordsNoMarker(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, _ := createStubChat(t, h, imported, "streamstub")

	var result struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats/"+chatID+"/switch",
		map[string]string{"provider": "streamstub"}, http.StatusOK, &result)
	h.Quiesce()

	assert.Empty(t, readInterruptions(t, h, imported, chatID))
}

func stubExchange(t *testing.T, h *harness, imported importedRepo, runnerID string) {
	t.Helper()
	base := repoBase(imported)
	postHook(t, h, base, "switchablestub", runnerID, "session_start", `{"session_id":"s-1"}`)
	postHook(t, h, base, "switchablestub", runnerID, "user_prompt", `{"prompt":"hi"}`)
	postHook(t, h, base, "switchablestub", runnerID, "turn_stop", `{"session_id":"s-1","last_assistant_message":"ok"}`)
	h.Quiesce()
}

func TestRegression_SelectionChangeRecordsModelAndEffortMarkers(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "switchablestub", switchableStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "switchablestub")
	stubExchange(t, h, imported, runnerID)

	resp := h.raw(http.MethodPatch, repoBase(imported)+"/chats/"+chatID+"/selection",
		map[string]string{"model": "opus", "effort": "high"}, http.StatusNoContent)
	_ = resp.Body.Close()
	h.Quiesce()

	interruptions := readInterruptions(t, h, imported, chatID)
	require.Len(t, interruptions, 2)
	kinds := []string{interruptions[0].Kind, interruptions[1].Kind}
	details := []string{interruptions[0].Detail, interruptions[1].Detail}
	assert.ElementsMatch(t, []string{"model_changed", "effort_changed"}, kinds)
	assert.ElementsMatch(t, []string{"opus", "high"}, details)
}

func TestRegression_SelectionChangeOfOnlyOneHalfRecordsOneMarker(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "switchablestub", switchableStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "switchablestub")
	stubExchange(t, h, imported, runnerID)

	resp := h.raw(http.MethodPatch, repoBase(imported)+"/chats/"+chatID+"/selection",
		map[string]string{"model": "", "effort": "low"}, http.StatusNoContent)
	_ = resp.Body.Close()
	h.Quiesce()

	interruptions := readInterruptions(t, h, imported, chatID)
	require.Len(t, interruptions, 1)
	assert.Equal(t, "effort_changed", interruptions[0].Kind)
	assert.Equal(t, "low", interruptions[0].Detail)
}

func TestRegression_SwitchingProvidersDropsAModelTheNewProviderDoesNotDeclare(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "switchablestub", switchableStubProviderDescriptorYAML)
	writeProviderDescriptor(t, h, "otherswitchablestub", otherSwitchableStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, _ := createStubChat(t, h, imported, "switchablestub")

	resp := h.raw(http.MethodPatch, repoBase(imported)+"/chats/"+chatID+"/selection",
		map[string]string{"model": "opus", "effort": "high"}, http.StatusNoContent)
	_ = resp.Body.Close()
	h.Quiesce()

	var result struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats/"+chatID+"/switch",
		map[string]string{"provider": "otherswitchablestub"}, http.StatusOK, &result)
	h.QuiesceReactors()

	runner, err := h.app.Usecases.AgentRunner.LiveRunnerForChat(context.Background(), chatID)
	require.NoError(t, err, "the switch must leave a live runner on the chat")
	assert.Empty(t, runner.LaunchModel,
		"otherswitchablestub does not declare \"opus\" — the switch must not hand it "+
			"the outgoing provider's model verbatim, or the new provider rejects the "+
			"spawn exactly like the reported model_not_found bug")
	assert.Empty(t, runner.LaunchEffort,
		"effort is validated per-model (Efforts(model)), so a cleared model must clear "+
			"the stale effort with it rather than leave a mismatched pair")
}

func TestRegression_AFailureFromBeforeTheSwitchSortsBeforeIt(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "streamstub")

	post := func(event, payload string) {
		postHook(t, h, repoBase(imported), "streamstub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"pick a model"}`)
	h.Quiesce()

	post("turn_failed", `{"session_id":"sess-1","error":"model_not_found",`+
		`"error_details":"gpt-5.4-mini may not exist or you may not have access to it"}`)
	h.Quiesce()

	var result struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats/"+chatID+"/switch",
		map[string]string{"provider": "quietstub"}, http.StatusOK, &result)
	h.Quiesce()

	rows := chatRows(t, h, chatID)
	failed := seqOf(t, rows, ledger.KindTurnFailed, func(ledger.TurnFailed) bool { return true })
	switched := seqOf(t, rows, ledger.KindMarker, func(b ledger.Marker) bool { return b.Kind == ledger.MarkerProviderSwitched })
	assert.Less(t, failed, switched, "the failure was recorded before the switch was requested")
}
