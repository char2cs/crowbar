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

// switchableStubProviderDescriptorYAML is streamStubProviderDescriptorYAML
// (see regression_agent_message_stream_test.go) plus a declared model/effort
// catalogue: SetChatSelection's validateSelection refuses a value outside
// the provider's own available list, and streamstub/quietstub declare none —
// only the selection-marker tests below need one.
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

// otherSwitchableStubProviderDescriptorYAML is switchableStubProviderDescriptorYAML
// under a different id and DISJOINT model/effort catalogues — neither
// sonnet/opus nor low/high appear here — so a stale value surviving the
// switch from switchablestub is unambiguously wrong, not coincidentally
// still valid. See
// TestRegression_SwitchingProvidersDropsAModelTheNewProviderDoesNotDeclare.
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

// readInterruptions are the markers chatID's feed records.
func readInterruptions(t *testing.T, h *harness, _ importedRepo, chatID string) []ledger.Marker {
	t.Helper()
	return rowsOf(t, chatRows(t, h, chatID), ledger.KindMarker, func(ledger.Marker) bool { return true })
}

// The real POST .../switch records the marker the transcript draws its
// divider from, on the chat's feed.
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
	h.post(repoBase(imported)+"/chats/"+chatID+"/switch",
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

// TestRegression_SelectionChangeOfOnlyOneHalfRecordsOneMarker proves the
// halves are independent: changing just the effort (model left as-is) must
// not ALSO draw a model-changed pill for a value that never actually moved.
func TestRegression_SelectionChangeOfOnlyOneHalfRecordsOneMarker(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "switchablestub", switchableStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, _ := createStubChat(t, h, imported, "switchablestub")

	resp := h.raw(http.MethodPatch, repoBase(imported)+"/chats/"+chatID+"/selection",
		map[string]string{"model": "", "effort": "low"}, http.StatusNoContent)
	_ = resp.Body.Close()
	h.Quiesce()

	interruptions := readInterruptions(t, h, imported, chatID)
	require.Len(t, interruptions, 1)
	assert.Equal(t, "effort_changed", interruptions[0].Kind)
	assert.Equal(t, "low", interruptions[0].Detail)
}

// TestRegression_SwitchingProvidersDropsAModelTheNewProviderDoesNotDeclare is
// the actual reported bug behind a screenshot: a chat on codex with model
// "gpt-5.4-mini" switched to claude, and claude was spawned with THAT SAME
// model string verbatim — invalid for claude, so it rejected the turn with
// "model_not_found". The switch's OWN divider claimed the new selection was
// claude's default the whole time, because that marker comes from a separate,
// later write that never reaches the CLI already spawned (and already dead)
// by then. ChatSelection reads the chat's raw stored model/effort for a
// switch (unlike a fresh mint, which reads them as "the provider's own
// default") — nothing validated that value against the INCOMING provider
// before handing it to spawn's argv.
func TestRegression_SwitchingProvidersDropsAModelTheNewProviderDoesNotDeclare(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "switchablestub", switchableStubProviderDescriptorYAML)
	writeProviderDescriptor(t, h, "otherswitchablestub", otherSwitchableStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, _ := createStubChat(t, h, imported, "switchablestub")

	// "opus" is valid for switchablestub, and invalid for otherswitchablestub
	// (which only declares "haiku") — the exact shape of the live bug.
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

// A turn that failed before a switch is recorded before the switch's marker,
// so the old provider's error is never drawn as the new session's.
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
