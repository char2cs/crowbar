//go:build integration

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

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

// otherSwitchableStubProviderDescriptorYAML is switchableStubProviderDescriptorYAML
// under a different id and DISJOINT model/effort catalogues — neither
// sonnet/opus nor low/high appear here — so a stale value surviving the
// switch from switchablestub is unambiguously wrong, not coincidentally
// still valid. See
// TestRegression_SwitchingProvidersDropsAModelTheNewProviderDoesNotDeclare.
const otherSwitchableStubProviderDescriptorYAML = `id: otherswitchablestub
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
	resp := h.raw(http.MethodPatch, wsBase(imported)+"/chats/"+chatID+"/selection",
		map[string]string{"model": "opus", "effort": "high"}, http.StatusAccepted)
	_ = resp.Body.Close()
	h.Quiesce()

	var result struct {
		ID string `json:"id"`
	}
	h.post(wsBase(imported)+"/chats/"+chatID+"/switch",
		map[string]string{"provider": "otherswitchablestub"}, http.StatusOK, &result)
	h.QuiesceReactors()

	runner, err := h.app.Repositories.AgentRunner.LiveRunnerForChat(context.Background(), chatID)
	require.NoError(t, err, "the switch must leave a live runner on the chat")
	assert.Empty(t, runner.LaunchModel,
		"otherswitchablestub does not declare \"opus\" — the switch must not hand it "+
			"the outgoing provider's model verbatim, or the new provider rejects the "+
			"spawn exactly like the reported model_not_found bug")
	assert.Empty(t, runner.LaunchEffort,
		"effort is validated per-model (Efforts(model)), so a cleared model must clear "+
			"the stale effort with it rather than leave a mismatched pair")
}

// orderedMessage is recordedMessage plus DisplayOrder, which the transcript
// actually sorts by — see TestRegression_AFailureFromBeforeTheSwitchSortsBeforeIt.
type orderedMessage struct {
	DisplayOrder int64  `json:"displayOrder"`
	Role         string `json:"role"`
	Text         string `json:"text"`
}

func readOrderedMessages(t *testing.T, h *harness, imported importedRepo, chatID string) []orderedMessage {
	t.Helper()
	var page struct {
		Items []orderedMessage `json:"items"`
	}
	h.get(wsBase(imported)+"/chats/"+chatID+"/messages?limit=200", &page)
	return page.Items
}

// orderedInterruption is interruptionDTO plus DisplayOrder.
type orderedInterruption struct {
	Kind         string `json:"kind"`
	Detail       string `json:"detail"`
	DisplayOrder int64  `json:"displayOrder"`
}

func readOrderedInterruptions(t *testing.T, h *harness, imported importedRepo, chatID string) []orderedInterruption {
	t.Helper()
	var activity struct {
		Interruptions []orderedInterruption `json:"interruptions"`
	}
	h.get(wsBase(imported)+"/chats/"+chatID+"/activity", &activity)
	return activity.Interruptions
}

// TestRegression_AFailureFromBeforeTheSwitchSortsBeforeIt is the reported bug
// behind a screenshot: a codex turn failed with "model_not_found", the chat
// went idle, the user then switched to claude — and the transcript rendered
// the OLD codex failure notice AFTER the "Switched to Claude"/"Model: haiku"
// divider pair, as if it belonged to the new session. The failure and the
// switch are not concurrent here — the failure is fully recorded, and the
// chat is fully idle, before the switch is even requested — so if THIS
// still sorts wrong, the bug is in how DisplayOrder is computed or compared,
// not in a race between an async hook and a synchronous switch.
func TestRegression_AFailureFromBeforeTheSwitchSortsBeforeIt(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "streamstub")

	post := func(event, payload string) {
		postProviderHook(t, h, imported, "streamstub", runnerID, event, payload)
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
	h.post(wsBase(imported)+"/chats/"+chatID+"/switch",
		map[string]string{"provider": "quietstub"}, http.StatusOK, &result)
	h.Quiesce()

	messages := readOrderedMessages(t, h, imported, chatID)
	var notice *orderedMessage
	for i := range messages {
		if messages[i].Role == "notice" {
			notice = &messages[i]
		}
	}
	require.NotNil(t, notice, "the codex failure must be recorded as a notice-role message")

	interruptions := readOrderedInterruptions(t, h, imported, chatID)
	var switchMarker *orderedInterruption
	for i := range interruptions {
		if interruptions[i].Kind == "provider_switched" {
			switchMarker = &interruptions[i]
		}
	}
	require.NotNil(t, switchMarker, "the switch must record a provider_switched marker")

	assert.Less(t, notice.DisplayOrder, switchMarker.DisplayOrder,
		"the codex failure happened, and was fully recorded, BEFORE the switch was even "+
			"requested — its DisplayOrder must sort before the switch marker's, or the "+
			"transcript renders the old provider's error as if it belonged to the new session")
}

// waitingForTurnLog duplicates turn.WaitingForTurnLog's literal text: that
// constant lives under .../usecases/chat/internal/turn, and Go's internal
// package boundary makes it unreachable from this black-box package. It is
// the one CAUSAL signal a test can block on to know a switch has genuinely
// parked on an in-flight turn (see parkedOnSwitchWait) — see the constant's
// own doc comment at the source for why nothing weaker will do.
const waitingForTurnLog = "agent: switch provider: the chat is mid-turn; waiting for the CLI to finish before quitting it"

// parkedOnSwitchWait returns a channel closed the moment a switch parks on an
// in-flight turn — the usecase's own log record, emitted immediately before it
// blocks. It is what lets TestRegression_AConcurrentFailureSortsBeforeTheSwitch
// fire the failure hook at a moment it KNOWS the switch is genuinely waiting,
// rather than guessing with a sleep.
func parkedOnSwitchWait(t *testing.T) <-chan struct{} {
	t.Helper()
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := &parkOnLogHandler{ch: make(chan struct{}), match: waitingForTurnLog}
	slog.SetDefault(slog.New(h))
	return h.ch
}

type parkOnLogHandler struct {
	once  sync.Once
	ch    chan struct{}
	match string
}

func (h *parkOnLogHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *parkOnLogHandler) Handle(_ context.Context, r slog.Record) error {
	if strings.Contains(r.Message, h.match) {
		h.once.Do(func() { close(h.ch) })
	}
	return nil
}

func (h *parkOnLogHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *parkOnLogHandler) WithGroup(string) slog.Handler      { return h }

// TestRegression_AConcurrentFailureSortsBeforeTheSwitch is the actual reported
// scenario, not the simplified sequential one above: the user switches provider
// WHILE codex is still mid-turn, and codex's "model_not_found" failure is what
// finally lets the switch's own wait release. The failure and the switch race
// for real here — RecordChatSwitch's Interrupt/ResolveInterruption use asynx's
// non-waiting Send, while the failure notice's AppendTurn uses SendWait — so
// this is the one scenario that can actually catch a Seq/DisplayOrder ordering
// bug the sequential test cannot.
func TestRegression_AConcurrentFailureSortsBeforeTheSwitch(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createStubChat(t, h, imported, "streamstub")

	post := func(event, payload string) {
		postProviderHook(t, h, imported, "streamstub", runnerID, event, payload)
	}
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"pick a model"}`)
	h.Quiesce()

	parked := parkedOnSwitchWait(t)

	type switchResult struct {
		status int
		body   []byte
	}
	switchDone := make(chan switchResult, 1)
	go func() {
		body, _ := json.Marshal(map[string]string{"provider": "quietstub"})
		req, err := http.NewRequest(http.MethodPost,
			h.url+wsBase(imported)+"/chats/"+chatID+"/switch", bytes.NewReader(body))
		if err != nil {
			switchDone <- switchResult{status: -1}
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := h.server.Client().Do(req)
		if err != nil {
			switchDone <- switchResult{status: -1}
			return
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		switchDone <- switchResult{status: resp.StatusCode, body: respBody}
	}()

	select {
	case <-parked:
	case <-time.After(10 * time.Second):
		t.Fatal("switch never parked on the in-flight turn — nothing to race against")
	}

	post("turn_failed", `{"session_id":"sess-1","error":"model_not_found",`+
		`"error_details":"gpt-5.4-mini may not exist or you may not have access to it"}`)

	select {
	case result := <-switchDone:
		require.Equal(t, http.StatusOK, result.status, "switch response: %s", result.body)
	case <-time.After(10 * time.Second):
		t.Fatal("switch never returned after the parked turn failed")
	}
	h.Quiesce()

	messages := readOrderedMessages(t, h, imported, chatID)
	var notice *orderedMessage
	for i := range messages {
		if messages[i].Role == "notice" {
			notice = &messages[i]
		}
	}
	require.NotNil(t, notice, "the codex failure must be recorded as a notice-role message")

	interruptions := readOrderedInterruptions(t, h, imported, chatID)
	var switchMarker *orderedInterruption
	for i := range interruptions {
		if interruptions[i].Kind == "provider_switched" {
			switchMarker = &interruptions[i]
		}
	}
	require.NotNil(t, switchMarker, "the switch must record a provider_switched marker")

	assert.Less(t, notice.DisplayOrder, switchMarker.DisplayOrder,
		"the codex failure is what RELEASED the switch's own wait — it happened first "+
			"by construction — so its DisplayOrder must sort before the switch marker's")
}
