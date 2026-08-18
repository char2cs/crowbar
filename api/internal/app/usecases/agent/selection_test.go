package agent_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/agent"
)

// writeDescriptor drops an on-disk descriptor override into the fixture's crowbar
// home, which is exactly the channel a user overrides a provider through.
func writeDescriptor(t *testing.T, f testFixture, id, body string) {
	t.Helper()
	dir := filepath.Join(f.ws.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".yaml"), []byte(body), 0o600))
}

// rewakeDescriptorBody is a provider whose prompt delivery does NOT restart the
// TUI (strategy: rewake_hook) while its model/effort blocks do. It is the shape
// that separates the two authorities on restarting: nothing about delivering a
// message respawns this CLI, so any restart it gets is the selection's doing.
const rewakeDescriptorBody = `
id: claude
display_name: Rewake
spawn:
  cmd: claude
  interactive_required: true
  forbid_flags: ["-p", "--print"]
session:
  resume: { arg: "--resume {id}" }
presentation:
  prompt_submit:
    strategy: rewake_hook
    rewake: { sentinel: "crowbar-delivered" }
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
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    user_prompt:   { message: prompt }
    turn_stop:     { message: last_assistant_message }
`

// silentDescriptorBody declares no model and no effort block at all: the shape a
// provider Crowbar knows nothing about takes, and the one that must cost a spawn
// nothing.
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
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    user_prompt:   { message: prompt }
    turn_stop:     { message: last_assistant_message }
`

func TestSetChatSelection_WritesADeclaredChoice(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", "high"))

	chat := f.chat(t, chatID)
	assert.Equal(t, "opus", chat.Model)
	assert.Equal(t, "high", chat.Effort)
}

// TestSetChatSelection_ClearsBackToTheProviderDefault: empty is a legitimate
// selection — "whatever this provider defaults to" — and must be writable, or a
// user who picks a model can never un-pick it.
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

// TestSetChatSelection_RefusesWhereTheProviderDeclaresNoCatalogue: a provider
// that declares no models offers no picker, so every value is outside its
// catalogue. It is driven against a synthetic descriptor because both shipped
// providers now declare one.
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

// TestSetChatSelection_ValidatesTheEffortAgainstTheIncomingModel: both halves
// move in one call, so an effort that is only valid under the model being SET
// must be judged against that model rather than the stored one.
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
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop:     { message: last_assistant_message }
`)
	chatID, _ := f.spawn(t, "claude")

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", "max"))
	err := f.usecase.SetChatSelection(f.ctx, chatID, "sonnet", "max")

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

// TestSpawn_UnselectedChatSpawnsIdenticalArgv is the inert-path guarantee at the
// usecase level: the whole feature must cost a chat that uses none of it exactly
// nothing, argv for argv.
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
	// Everything the baseline spawn carried is still carried, in order, ahead of
	// the prompt-delivery steps the replacement adds.
	require.GreaterOrEqual(t, len(replacement), len(baseline))
	assert.Equal(t, baseline[:2], replacement[:2])
}

// TestSpawn_CarriesTheSelectionIntoTheArgvAndRecordsItOnTheRunner walks the whole
// path: a stored choice reaches the process, and what the process was launched
// with is recorded — the record being the only thing that can ever answer "what
// is this CLI running", since no CLI will tell us.
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

// TestSubmitPrompt_ASelectionSwitchForcesARestartADeliveryWouldNotHaveDone is the
// point of the model/effort block's own strategy.
//
// The provider here delivers prompts WITHOUT respawning (rewake_hook), so the
// delivery strategy alone refuses the message — Crowbar implements no such
// channel. A pending selection switch authorises the restart on its own, and the
// same message then goes through.
func TestSubmitPrompt_ASelectionSwitchForcesARestartADeliveryWouldNotHaveDone(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", rewakeDescriptorBody)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "no switch pending", uuid.NewString())
	require.ErrorIs(t, err, agentusecase.ErrPromptUnsupported,
		"a delivery that never respawns has no channel here")

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", ""))
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "switch pending", uuid.NewString())

	require.NoError(t, err, "a pending selection switch obliges a restart on its own")
	call := f.term.calls[f.term.callCount()-1]
	modelAt := indexOf(call.argv, "--model")
	require.GreaterOrEqual(t, modelAt, 0)
	assert.Equal(t, "opus", call.argv[modelAt+1])
}

// TestSubmitPrompt_TheRestartResumesTheNativeConversation: a forced restart must
// not cost the user their conversation. The replacement carries the native
// session id it was resuming, exactly as an ordinary prompt restart does.
func TestSubmitPrompt_TheRestartResumesTheNativeConversation(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", rewakeDescriptorBody)
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

// TestSubmitPrompt_ClearingBackToTheDefaultAlsoForcesTheRestart pins the
// direction that is easy to forget: the provider default is not any declared
// value, so returning to it needs a process launched without the flag.
func TestSubmitPrompt_ClearingBackToTheDefaultAlsoForcesTheRestart(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", rewakeDescriptorBody)
	chatID, _ := f.spawn(t, "claude")
	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", ""))
	first, err := f.usecase.SubmitPrompt(f.ctx, chatID, "under opus", uuid.NewString())
	require.NoError(t, err)
	// The CLI acknowledging the delivered prompt is what releases the durable
	// dispatch barrier; without it the chat is legitimately busy for the next one.
	require.NoError(t, f.usecase.IngestHook(f.ctx, first.RunnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "under opus"})))
	turn(t, f, first.RunnerID, "claude", "answered")

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "", ""))
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "back to default", uuid.NewString())

	require.NoError(t, err)
	call := f.term.calls[f.term.callCount()-1]
	assert.Less(t, indexOf(call.argv, "--model"), 0, "the default carries no model flag")
}

// TestSubmitPrompt_NoSwitchUnderARestartingDeliveryIsUnchanged keeps the shipped
// providers on exactly their old path: claude's delivery restarts for every
// message, so the selection machinery never has to be consulted for a chat that
// selected nothing.
func TestSubmitPrompt_NoSwitchUnderARestartingDeliveryIsUnchanged(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "ordinary message", uuid.NewString())

	require.NoError(t, err)
	assert.Equal(t, 2, f.term.callCount(), "the original spawn plus the delivery restart")
}

// TestResolveProviders_PublishesTheCatalogueAndItsAbsence is what lets a client
// render a picker with no hardcoded provider knowledge: the levels are resolved
// per model server-side, so a client reads efforts[model] and is done.
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

	// codex's catalogue is per-model with NO fallback key, so its levels differ by
	// model and its default model — the "" key — has none at all. The absent key is
	// the point: a null entry would invite a client to render an empty picker.
	codex := list[byID["codex"]]
	assert.True(t, codex.ModelSelect)
	assert.True(t, codex.EffortSelect)
	assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max", "ultra"},
		codex.Efforts["gpt-5.6-sol"])
	assert.Equal(t, []string{"low", "medium", "high", "xhigh"}, codex.Efforts["gpt-5.4"])
	assert.NotContains(t, codex.Efforts, "")
}

// TestResolveProviders_AProviderDeclaringNothingReportsNoCatalogue is the absent
// case, driven against a synthetic descriptor now that both shipped providers
// declare one.
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

// TestSetChatSelection_ADormantChatIsJudgedByItsLastProvider: liveness is a
// query, and a chat whose CLI has exited still has a provider — the one its last
// conversation was with. The picker on a dormant chat must therefore still
// validate, or reopening a chat would be the only way to change its model.
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

// TestSetChatSelection_AChatNoProviderHasEverRunOnIsUnprocessable: a minted chat
// with no runner has no catalogue to judge against. Storing a value nothing can
// validate is worse than refusing one no picker could have produced.
func TestSetChatSelection_AChatNoProviderHasEverRunOnIsUnprocessable(t *testing.T) {
	f := newFixture(t)
	chatID, err := f.usecase.MintChat(f.ctx, "ws1")
	require.NoError(t, err)
	f.wait()

	err = f.usecase.SetChatSelection(f.ctx, chatID, "opus", "")

	require.ErrorIs(t, err, apperr.ErrUnprocessable)
	// Clearing is still accepted: it asks for the default, which needs no
	// catalogue to be meaningful.
	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "", ""))
}

// TestSetChatSelection_SaveFailureSurfaces keeps a failed write from reading as a
// successful one: the picker must not show a value the aggregate never took.
func TestSetChatSelection_SaveFailureSurfaces(t *testing.T) {
	f, cs, _ := newFaultFixture(t)
	chatID, _ := f.spawn(t, "claude")
	cs.failSetSelection = errors.New("boom: save selection")

	err := f.usecase.SetChatSelection(f.ctx, chatID, "opus", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "save")
}

// TestSpawn_AnUnreadableSelectionFailsBeforeTheFork: the selection is resolved
// beside the lineage read and ahead of every side effect, so a spawn that cannot
// learn what to run as never creates a tmp dir or a process to unwind.
func TestSpawn_AnUnreadableSelectionFailsBeforeTheFork(t *testing.T) {
	f, cs, _ := newFaultFixture(t)
	chatID, _ := f.spawn(t, "claude")
	spawnsBefore := f.term.callCount()
	// The lineage read folds the chat first; the selection read is the one after
	// it, and it is the one under test.
	cs.failLoadChat = errors.New("boom: load chat")
	cs.failLoadChatAfter = 1

	_, err := f.usecase.StartRunner(f.ctx, chatID, "claude")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat selection")
	assert.Equal(t, spawnsBefore, f.term.callCount(), "no process may exist after this failure")
}

// TestSubmitPrompt_ATerminalOnlyProviderIsUnsupported: a provider declaring no
// chat-side prompt delivery is refused for THAT reason, ahead of anything the
// chat's selection has to say — the two refusals share a sentinel and must not
// share a cause.
func TestSubmitPrompt_ATerminalOnlyProviderIsUnsupported(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", `
id: claude
spawn:
  cmd: claude
  interactive_required: true
session:
  resume: { arg: "--resume {id}" }
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop:     { message: last_assistant_message }
`)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "nowhere to go", uuid.NewString())

	require.ErrorIs(t, err, agentusecase.ErrPromptUnsupported)
}

// TestSubmitPrompt_AnUnreadableSelectionRefusesTheDelivery: the restart decision
// cannot be taken without knowing what the chat wants, and guessing would deliver
// the message under the wrong model.
func TestSubmitPrompt_AnUnreadableSelectionRefusesTheDelivery(t *testing.T) {
	f, cs, _ := newFaultFixture(t)
	writeDescriptor(t, f, "claude", rewakeDescriptorBody)
	chatID, _ := f.spawn(t, "claude")
	cs.failLoadChat = errors.New("boom: load chat")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "which model?", uuid.NewString())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat selection")
}
