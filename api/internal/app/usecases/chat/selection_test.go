package chat_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func writeDescriptor(t *testing.T, f testFixture, id, body string) {
	t.Helper()
	dir := filepath.Join(f.ws.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".yaml"), []byte(body), 0o600))
}

const selectingDescriptorBody = `
id: claude
display_name: Selecting
spawn:
  cmd: claude
  interactive_required: true
  forbid_flags: ["-p", "--print"]
session:
  resume: { arg: "--resume {id}" }
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh:
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "{message}" }
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
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
`

const silentDescriptorBody = `
id: claude
spawn:
  cmd: claude
  interactive_required: true
session:
  resume: { arg: "--resume {id}" }
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh:
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "{message}" }
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
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
`

func TestSetChatSelection_WritesADeclaredChoice(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", "high"))

	chat := f.chat(t, chatID)
	assert.Equal(t, "opus", chat.Model)
	assert.Equal(t, "high", chat.Effort)
}

func TestSetChatSelection_ClearsBackToTheProviderDefault(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", "high"))

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "", ""))

	chat := f.chat(t, chatID)
	assert.Empty(t, chat.Model)
	assert.Empty(t, chat.Effort)
}

func TestSetChatSelection_RefusesAValueOutsideTheDeclaredCatalogue(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	testCases := []struct {
		name    string
		model   string
		effort  string
		wantMsg string
	}{
		{"unknown model", "gpt-5", "", "declares no model"},
		{"unknown effort", "opus", "ludicrous", "declares no effort"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := f.usecase.SetChatSelection(f.ctx, chatID, tc.model, tc.effort)

			require.ErrorIs(t, err, apperr.ErrInvalidArgument)
			assert.Contains(t, err.Error(), tc.wantMsg)
			assert.Empty(t, f.chat(t, chatID).Model, "a refused write must store nothing")
		})
	}
}

func TestSetChatSelection_RefusesWhereTheProviderDeclaresNoCatalogue(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", silentDescriptorBody)
	chatID, _ := f.spawn(t, "claude")

	err := f.usecase.SetChatSelection(f.ctx, chatID, "gpt-5", "")

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

func TestSetChatSelection_UnknownChatIsNotFound(t *testing.T) {
	f := newFixture(t)

	err := f.usecase.SetChatSelection(f.ctx, "no-such-chat", "opus", "")

	require.Error(t, err)
}

func TestSetChatSelection_ValidatesTheEffortAgainstTheIncomingModel(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", `
id: claude
spawn:
  cmd: claude
  interactive_required: true
session:
  resume: { arg: "--resume {id}" }
model:
  available: [sonnet, opus]
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--model", value: "{model}" }
effort:
  available:
    "*": [low]
    opus: [max]
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--effort", value: "{effort}" }
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
`)
	chatID, _ := f.spawn(t, "claude")

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", "max"))
	err := f.usecase.SetChatSelection(f.ctx, chatID, "sonnet", "max")

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

func TestSpawn_UnselectedChatSpawnsIdenticalArgv(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	baseline := f.term.calls[0].argv

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "", ""))
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "hello", uuid.NewString())
	require.NoError(t, err)

	replacement := f.term.calls[f.term.callCount()-1].argv
	for _, arg := range replacement {
		assert.NotEqual(t, "--model", arg)
		assert.NotEqual(t, "--effort", arg)
	}

	require.GreaterOrEqual(t, len(replacement), len(baseline))
	assert.Equal(t, baseline[:2], replacement[:2])
}

func TestSpawn_CarriesTheSelectionIntoTheArgvAndRecordsItOnTheRunner(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", "high"))

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "with a model", uuid.NewString())
	require.NoError(t, err)

	call := f.term.calls[f.term.callCount()-1]
	modelAt := indexOf(call.argv, "--model")
	require.GreaterOrEqual(t, modelAt, 0)
	assert.Equal(t, "opus", call.argv[modelAt+1])
	effortAt := indexOf(call.argv, "--effort")
	require.GreaterOrEqual(t, effortAt, 0)
	assert.Equal(t, "high", call.argv[effortAt+1])

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "opus", live.LaunchModel)
	assert.Equal(t, "high", live.LaunchEffort)
}

type undeliverableAgent struct {
	engineagents.Agent
}

func (undeliverableAgent) Capabilities() engineagents.Capabilities {
	return engineagents.Capabilities{PromptSubmit: true, Delivery: "telepathy"}
}

func TestSubmitPrompt_ASelectionSwitchAuthorisesARestartADeliveryWouldNotHaveDone(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", selectingDescriptorBody)
	chatID, _ := f.spawn(t, "claude")
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	shipped, err := f.engine.Get(f.ctx, f.ws.home, "claude")
	require.NoError(t, err)
	descriptor := undeliverableAgent{Agent: shipped}
	require.NotEqual(t, engineagents.DeliveryRestartTUI, descriptor.Capabilities().Delivery,
		"the fixture must not restart for delivery reasons, or this proves nothing")

	err = agentusecase.RequirePromptRestart(f.ctx, f.usecase.RunnerUsecase, chatID, live, descriptor)
	require.ErrorIs(t, err, agentusecase.ErrPromptUnsupported,
		"a delivery this daemon has no channel for is refused, never guessed at")

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", ""))

	require.NoError(t, agentusecase.RequirePromptRestart(f.ctx, f.usecase.RunnerUsecase, chatID, live, descriptor),
		"a pending selection switch obliges a restart on its own")
}

func TestSubmitPrompt_TheRestartResumesTheNativeConversation(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", selectingDescriptorBody)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "native-session")
	turn(t, f, runnerID, "claude", "the conversation exists")
	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", ""))

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "keep my history", uuid.NewString())

	require.NoError(t, err)
	call := f.term.calls[f.term.callCount()-1]
	resumeAt := indexOf(call.argv, "--resume")
	require.GreaterOrEqual(t, resumeAt, 0, "the forced restart must resume, not start fresh")
	assert.Equal(t, "native-session", call.argv[resumeAt+1])
}

func TestSubmitPrompt_ClearingBackToTheDefaultAlsoForcesTheRestart(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", selectingDescriptorBody)
	chatID, _ := f.spawn(t, "claude")
	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", ""))
	first, err := f.usecase.SubmitPrompt(f.ctx, chatID, "under opus", uuid.NewString())
	require.NoError(t, err)

	require.NoError(t, f.usecase.IngestHook(f.ctx, first.RunnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "under opus"})))
	turn(t, f, first.RunnerID, "claude", "answered")

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "", ""))
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "back to default", uuid.NewString())

	require.NoError(t, err)
	call := f.term.calls[f.term.callCount()-1]
	assert.Less(t, indexOf(call.argv, "--model"), 0, "the default carries no model flag")
}

func TestSubmitPrompt_NoSwitchUnderARestartingDeliveryIsUnchanged(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "ordinary message", uuid.NewString())

	require.NoError(t, err)
	assert.Equal(t, 2, f.term.callCount(), "the original spawn plus the delivery restart")
}

func TestResolveProviders_PublishesTheCatalogueAndItsAbsence(t *testing.T) {
	f := newFixture(t)

	list, err := f.usecase.ResolveProviders(f.ctx)
	require.NoError(t, err)

	byID := map[string]int{}
	for i, p := range list {
		byID[p.ID] = i
	}
	claude := list[byID["claude"]]
	assert.True(t, claude.ModelSelect)
	assert.True(t, claude.EffortSelect)
	assert.Equal(t, []string{"sonnet", "opus", "haiku"}, claude.Models)
	assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, claude.Efforts[""])
	for _, model := range claude.Models {
		assert.NotEmpty(t, claude.Efforts[model], "every selectable model must answer its levels")
	}

	codex := list[byID["codex"]]
	assert.True(t, codex.ModelSelect)
	assert.True(t, codex.EffortSelect)
	assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max", "ultra"},
		codex.Efforts["gpt-5.6-sol"])
	assert.Equal(t, []string{"low", "medium", "high", "xhigh"}, codex.Efforts["gpt-5.4"])
	assert.NotContains(t, codex.Efforts, "")
}

func TestResolveProviders_AProviderDeclaringNothingReportsNoCatalogue(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", silentDescriptorBody)

	list, err := f.usecase.ResolveProviders(f.ctx)
	require.NoError(t, err)

	for _, p := range list {
		if p.ID != "claude" {
			continue
		}
		assert.False(t, p.ModelSelect)
		assert.False(t, p.EffortSelect)
		assert.Empty(t, p.Models)
		assert.Nil(t, p.Efforts)
		return
	}
	t.Fatal("the overridden provider must still be listed")
}

func TestSetChatSelection_ADormantChatIsJudgedByItsLastProvider(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "native-session")
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()
	_, err := f.liveRunnerFor(t, chatID)
	require.Error(t, err, "the chat must be dormant for this to prove anything")

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", ""))

	assert.Equal(t, "opus", f.chat(t, chatID).Model)
}

func TestSetChatSelection_AChatNoProviderHasEverRunOnIsUnprocessable(t *testing.T) {
	f := newFixture(t)
	chatID, err := f.usecase.MintChat(f.ctx, "ws1")
	require.NoError(t, err)
	f.wait()

	err = f.usecase.SetChatSelection(f.ctx, chatID, "opus", "")

	require.ErrorIs(t, err, apperr.ErrUnprocessable)

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "", ""))
}

func TestSetChatSelection_SaveFailureSurfaces(t *testing.T) {
	f, cs, _ := newFaultFixture(t)
	chatID, _ := f.spawn(t, "claude")
	cs.failSetSelection = errors.New("boom: save selection")

	err := f.usecase.SetChatSelection(f.ctx, chatID, "opus", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "save")
}

func TestSpawn_AnUnreadableSelectionFailsBeforeTheFork(t *testing.T) {
	f, cs, _ := newFaultFixture(t)
	chatID, _ := f.spawn(t, "claude")
	spawnsBefore := f.term.callCount()

	cs.failLoadChat = errors.New("boom: load chat")
	cs.failLoadChatAfter = 1

	_, err := f.usecase.StartRunner(f.ctx, chatID, "claude")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat selection")
	assert.Equal(t, spawnsBefore, f.term.callCount(), "no process may exist after this failure")
}

func TestSubmitPrompt_ATerminalOnlyProviderIsUnsupported(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", `
id: claude
spawn:
  cmd: claude
  interactive_required: true
session:
  resume: { arg: "--resume {id}" }
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
`)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "nowhere to go", uuid.NewString())

	require.ErrorIs(t, err, agentusecase.ErrPromptUnsupported)
}

func TestSubmitPrompt_AnUnreadableSelectionRefusesTheDelivery(t *testing.T) {
	f, cs, _ := newFaultFixture(t)
	writeDescriptor(t, f, "claude", selectingDescriptorBody)
	chatID, _ := f.spawn(t, "claude")
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	shipped, err := f.engine.Get(f.ctx, f.ws.home, "claude")
	require.NoError(t, err)
	cs.failLoadChat = errors.New("boom: load chat")

	err = agentusecase.RequirePromptRestart(
		f.ctx, f.usecase.RunnerUsecase, chatID, live, undeliverableAgent{Agent: shipped},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat selection")
}
