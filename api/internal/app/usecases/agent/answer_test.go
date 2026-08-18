package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// deliver drives a hook through the RELAY ingress — the one that carries a
// delivery id — because the delivery id is the key a blocked relay is answered
// on. IngestHook (no id) is the daemon's own replay path and can hold nothing.
func deliver(
	t *testing.T,
	f testFixture,
	runnerID, provider, kind string,
	payload map[string]any,
) string {
	t.Helper()
	deliveryID := uuid.NewString()
	require.NoError(t, f.usecase.IngestHookDelivery(
		f.ctx, "ws1", deliveryID, runnerID, provider, kind, mustJSON(t, payload)))
	f.wait()
	return deliveryID
}

// askUserQuestionPermission is the PermissionRequest captured live from claude
// 2.1.234 on 2026-08-18 for an AskUserQuestion call. Both PreToolUse and
// PermissionRequest fire for it, in that order, and the PermissionRequest is the
// one that can be ANSWERED — measured by returning the shape below and watching
// PostToolUse come back carrying the picked label, with no dialog drawn.
func askUserQuestionPermission() map[string]any {
	return map[string]any{
		"session_id": "s1", "prompt_id": "2819fe04",
		"hook_event_name": "PermissionRequest", "tool_name": "AskUserQuestion",
		"tool_input": map[string]any{
			"questions": []any{map[string]any{
				"question": "Which option do you prefer?", "header": "Choice",
				"options": []any{
					map[string]any{"label": "Option A", "description": "the first"},
					map[string]any{"label": "Option B", "description": "the second"},
				},
				"multiSelect": false,
			}},
		},
	}
}

// blockedPermission opens a prompt through the relay ingress and returns the
// delivery id the relay would be waiting on, plus the prompt itself.
func blockedPermission(
	t *testing.T,
	f testFixture,
	chatID, runnerID string,
	payload map[string]any,
) (deliveryID string, choice domain.ActivityChoice) {
	t.Helper()
	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	deliveryID = deliver(t, f, runnerID, "claude", engineagents.HookPermission, payload)

	pending := pendingChoices(t, f, chatID)
	require.Len(t, pending, 1)
	return deliveryID, pending[0]
}

// await runs the relay's long-poll on its own goroutine and hands back a channel
// carrying exactly what the relay would print.
func await(f testFixture, deliveryID string) <-chan string {
	out := make(chan string, 1)
	go func() {
		answer, err := f.usecase.AwaitAnswer(f.ctx, deliveryID)
		if err != nil {
			out <- "ERR: " + err.Error()
			return
		}
		out <- string(answer.Stdout)
	}()
	return out
}

// The property this whole channel exists for: a human answers in Crowbar's chat
// and the vendor CLI is told, with nobody touching the terminal.
func TestAnswer_APermissionIsAnsweredFromCrowbarAndReachesTheCLI(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	deliveryID, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	pending, waiting := f.usecase.PendingAnswer(deliveryID)
	require.True(t, waiting, "a claude permission must hold its relay open")
	assert.Equal(t, choice.ID, pending.ChoiceID)
	assert.Positive(t, pending.Wait, "the relay is given a finite budget, never an open one")

	printed := await(f, deliveryID)
	require.NoError(t, f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{"allow"}, "", nil))
	f.wait()

	// The exact wrapped shape measured against claude 2.1.234. Without the
	// hookSpecificOutput wrapper the CLI rejects the document outright.
	assert.JSONEq(t,
		`{"hookSpecificOutput":{"hookEventName":"PermissionRequest",`+
			`"decision":{"behavior":"allow"}}}`,
		<-printed)
	assert.Empty(t, pendingChoices(t, f, chatID), "an answered prompt stops pending")
}

func TestAnswer_ADenyCarriesTheHumansReasonToTheCLI(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	deliveryID, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	printed := await(f, deliveryID)
	require.NoError(t, f.usecase.AnswerChoice(
		f.ctx, chatID, choice.ID, []string{"deny"}, "not on this branch", nil))
	f.wait()

	assert.JSONEq(t,
		`{"hookSpecificOutput":{"hookEventName":"PermissionRequest",`+
			`"decision":{"behavior":"deny","message":"not on this branch"}}}`,
		<-printed)
}

// AskUserQuestion is answered by handing the tool its own input back with an
// `answers` object keyed by the question's TEXT — measured, and the reason the
// question vocabulary stays mapped on the permission event.
func TestAnswer_AQuestionIsAnsweredByEchoingTheToolInputWithThePick(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	deliveryID, choice := blockedPermission(t, f, chatID, runnerID, askUserQuestionPermission())
	require.Equal(t, domain.ChoiceKindQuestion, choice.Kind)
	require.Len(t, choice.Options, 2)

	printed := await(f, deliveryID)
	require.NoError(t, f.usecase.AnswerChoice(
		f.ctx, chatID, choice.ID, []string{choice.Options[1].ID}, "", nil))
	f.wait()

	var decoded struct {
		HookSpecificOutput struct {
			Decision struct {
				Behavior     string `json:"behavior"`
				UpdatedInput struct {
					Questions []map[string]any  `json:"questions"`
					Answers   map[string]string `json:"answers"`
				} `json:"updatedInput"`
			} `json:"decision"`
		} `json:"hookSpecificOutput"`
	}
	require.NoError(t, json.Unmarshal([]byte(<-printed), &decoded))
	updated := decoded.HookSpecificOutput.Decision.UpdatedInput
	assert.Equal(t, "allow", decoded.HookSpecificOutput.Decision.Behavior)
	require.Len(t, updated.Questions, 1, "the CLI must get its own question back")
	assert.Equal(t, "Option B", updated.Answers["Which option do you prefer?"])
}

// A provider that declares no answer channel must be completely unaffected. Codex
// observes permissions — it maps the tool vocabulary — but declares no way to be
// told a decision, so nothing is held and the relay exits exactly as it always
// did.
func TestRegression_AProviderWithNoAnswerChannelHoldsNoRelay(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")

	hook(t, f, runnerID, "codex", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	deliveryID := deliver(t, f, runnerID, "codex", engineagents.HookPermission, map[string]any{
		"session_id": "s1", "tool_name": "Bash",
		"tool_input": map[string]any{"command": "touch PROOF"},
	})

	pending := pendingChoices(t, f, chatID)
	require.Len(t, pending, 1, "the prompt is still OBSERVED, exactly as before")
	_, waiting := f.usecase.PendingAnswer(deliveryID)
	assert.False(t, waiting, "a provider with no answer channel must never block its relay")
	assert.Empty(t, f.usecase.AnswerableChoiceIDs(chatID, pending),
		"and its prompt must not advertise a button that would reach nobody")

	// Answering it is refused rather than silently recorded: there is no relay to
	// print to, so success would be a lie.
	err := f.usecase.AnswerChoice(f.ctx, chatID, pending[0].ID, []string{"allow"}, "", nil)
	require.ErrorIs(t, err, apperr.ErrConflict)
}

// Every other hook of an answering provider is unaffected too — only the events
// its descriptor names can hold a relay.
func TestRegression_AnOrdinaryHookNeverHoldsItsRelay(t *testing.T) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	deliveryID := deliver(t, f, runnerID, "claude", engineagents.HookToolPre, map[string]any{
		"session_id": "s1", "tool_use_id": "tool-1", "tool_name": "Bash",
		"tool_input": map[string]any{"command": "ls"},
	})

	_, waiting := f.usecase.PendingAnswer(deliveryID)
	assert.False(t, waiting,
		"tool_pre fires for EVERY call; holding it would stall work nobody was asked about")
}

// A payload too large to echo back is observed and not held. Answering it would
// mean printing an unbounded document on a hook's stdout.
func TestAnswer_AnOversizedPayloadIsObservedButNotHeld(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	payload := permissionPayload()
	payload["tool_input"] = map[string]any{"command": strings.Repeat("x", 200<<10)}
	deliveryID := deliver(t, f, runnerID, "claude", engineagents.HookPermission, payload)

	require.Len(t, pendingChoices(t, f, chatID), 1)
	_, waiting := f.usecase.PendingAnswer(deliveryID)
	assert.False(t, waiting)
}

// The SIGTERM report. A human who says NO at the PTY fires no hook at all
// (measured against claude 2.1.234: no PostToolUse, no PostToolUseFailure, no
// PermissionDenied, no Stop) — the blocked relay is simply killed. This report is
// the only thing that stops that question hanging in the chat.
func TestAnswer_AbandonClearsAPromptDecidedAtTheTerminal(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	deliveryID, _ := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	printed := await(f, deliveryID)
	require.NoError(t, f.usecase.AbandonAnswer(f.ctx, deliveryID))
	f.wait()

	assert.Empty(t, <-printed, "a prompt decided elsewhere prints NOTHING")
	assert.Empty(t, pendingChoices(t, f, chatID),
		"and stops being a question the chat says nobody has answered")
}

func TestAnswer_AbandonOfAnUnknownDeliveryIsANoOp(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.usecase.AbandonAnswer(f.ctx, uuid.NewString()))
}

// Hooks are spawned DETACHED (measured against claude 2.1.234): killing a CLI
// leaves its hooks running as orphans, still blocked. A dead runner therefore has
// to release them itself — and clear its questions, because nothing else ever
// will.
func TestAnswer_ADeadRunnerReleasesEveryRelayAndClearsItsPrompts(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	deliveryID, _ := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	printed := await(f, deliveryID)
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	assert.Empty(t, <-printed, "an orphaned relay is released with no verdict")
	assert.Empty(t, pendingChoices(t, f, chatID),
		"a question asked by a process that no longer exists must not stay pending")
	_, waiting := f.usecase.PendingAnswer(deliveryID)
	assert.False(t, waiting)
}

func TestAnswer_RefusesAnOptionThePromptNeverOffered(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	_, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	err := f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{"not-an-option"}, "", nil)
	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

func TestAnswer_RefusesAnEmptyDecision(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	_, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	err := f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, nil, "", nil)
	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

// Two picks of different kinds are not one answer: there is no template that
// could render "allow AND deny", and inventing one would tell the CLI something
// nobody chose.
func TestAnswer_RefusesPicksOfDifferentKinds(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	_, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	err := f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{"allow", "deny"}, "", nil)
	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

// claude's permission_suggestions ride a channel that was never measured, so no
// template is declared for them. Refusing is the honest answer: a broader grant
// that silently narrowed to a plain allow would grant less than the user chose
// while reporting success.
func TestAnswer_RefusesADecisionTheProviderCannotExpress(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	_, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	suggestion := ""
	for _, option := range choice.Options {
		if option.Kind == domain.ChoiceOptionSuggestion {
			suggestion = option.ID
			break
		}
	}
	require.NotEmpty(t, suggestion, "claude's permission carries suggestions")

	err := f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{suggestion}, "", nil)
	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

func TestAnswer_RefusesAnUnknownChatOrPrompt(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	_, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	err := f.usecase.AnswerChoice(f.ctx, "no-such-chat", choice.ID, []string{"allow"}, "", nil)
	require.Error(t, err)

	err = f.usecase.AnswerChoice(f.ctx, chatID, "no-such-choice", []string{"allow"}, "", nil)
	require.ErrorIs(t, err, apperr.ErrConflict)
}

// A prompt can only be answered once. The second attempt finds no relay, which is
// exactly the state a prompt answered at the PTY is in.
func TestAnswer_ASecondAnswerToOnePromptIsRefused(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	deliveryID, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	printed := await(f, deliveryID)
	require.NoError(t, f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{"allow"}, "", nil))
	f.wait()
	<-printed

	err := f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{"deny"}, "", nil)
	require.ErrorIs(t, err, apperr.ErrConflict)
}

// A relay that has gone away must not leave a slot behind that a later answer
// could be written into.
func TestAnswer_ARelayThatDisconnectsFreesItsPrompt(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	deliveryID, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	ctx, cancel := context.WithCancel(f.ctx)
	done := make(chan error, 1)
	go func() {
		_, err := f.usecase.AwaitAnswer(ctx, deliveryID)
		done <- err
	}()
	cancel()
	require.Error(t, <-done)

	_, waiting := f.usecase.PendingAnswer(deliveryID)
	assert.False(t, waiting)
	err := f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{"allow"}, "", nil)
	require.ErrorIs(t, err, apperr.ErrConflict)
}

func TestAnswer_AwaitOnAnUnknownDeliveryReturnsNothingRatherThanFailing(t *testing.T) {
	f := newFixture(t)
	answer, err := f.usecase.AwaitAnswer(f.ctx, uuid.NewString())
	require.NoError(t, err)
	assert.Empty(t, answer.Stdout)
}

// claude's prompt_id is the TURN's id, not the prompt's: measured against 2.1.234
// on 2026-08-18, one turn's UserPromptSubmit, PreToolUse, PermissionRequest and
// Notification all carried the identical value. Keying identity on it alone gave
// every question in a turn ONE id, so a turn that asked about a Bash call and then
// about an Edit overwrote the first question with the second — and answering
// either would have answered whichever record survived.
func TestRegression_TwoPromptsInOneTurnDoNotShareAnIdentity(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})

	bash := permissionPayload()
	edit := permissionPayload()
	edit["tool_name"] = "Edit"
	edit["tool_input"] = map[string]any{"file_path": "/tmp/x"}
	require.Equal(t, bash["prompt_id"], edit["prompt_id"], "one turn, one prompt_id")

	deliver(t, f, runnerID, "claude", engineagents.HookPermission, bash)
	deliver(t, f, runnerID, "claude", engineagents.HookPermission, edit)

	pending := pendingChoices(t, f, chatID)
	require.Len(t, pending, 2, "two questions must be two records")
	assert.NotEqual(t, pending[0].ID, pending[1].ID)
}

// An MCP elicitation offers a SCHEMA, not a list, so its decision keys are the
// provider's own verbs. This is the whole reason the answer command accepts an id
// it cannot check against anything.
func TestAnswer_AnElicitationIsAnsweredWithAVerbAndAForm(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	deliveryID := deliver(t, f, runnerID, "claude", engineagents.HookElicitation, map[string]any{
		"session_id": "s1", "mcp_server_name": "spike", "mode": "form",
		"message":          "do you prefer A or B?",
		"requested_schema": map[string]any{"type": "object"},
	})

	pending := pendingChoices(t, f, chatID)
	require.Len(t, pending, 1)
	require.Empty(t, pending[0].Options, "an elicitation offers a schema, not options")

	printed := await(f, deliveryID)
	require.NoError(t, f.usecase.AnswerChoice(
		f.ctx, chatID, pending[0].ID, []string{"accept"}, "", []byte(`{"choice":"B"}`)))
	f.wait()

	assert.JSONEq(t,
		`{"hookSpecificOutput":{"hookEventName":"Elicitation",`+
			`"action":"accept","content":{"choice":"B"}}}`,
		<-printed)
}

// A multi-select question is answered with a LIST of labels, which is the shape
// the CLI produces for itself.
func TestAnswer_AMultiSelectQuestionCarriesEveryPickedLabel(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	payload := askUserQuestionPermission()
	questions, _ := payload["tool_input"].(map[string]any)["questions"].([]any)
	questions[0].(map[string]any)["multiSelect"] = true
	deliveryID, choice := blockedPermission(t, f, chatID, runnerID, payload)
	require.True(t, choice.Multi)

	printed := await(f, deliveryID)
	require.NoError(t, f.usecase.AnswerChoice(f.ctx, chatID, choice.ID,
		[]string{choice.Options[0].ID, choice.Options[1].ID}, "", nil))
	f.wait()

	var decoded struct {
		HookSpecificOutput struct {
			Decision struct {
				UpdatedInput struct {
					Answers map[string][]string `json:"answers"`
				} `json:"updatedInput"`
			} `json:"decision"`
		} `json:"hookSpecificOutput"`
	}
	require.NoError(t, json.Unmarshal([]byte(<-printed), &decoded))
	assert.Equal(t, []string{"Option A", "Option B"},
		decoded.HookSpecificOutput.Decision.UpdatedInput.Answers["Which option do you prefer?"])
}

// The COMMON race, measured: a human at the PTY always wins, immediately. Saying
// YES there completes the gated tool, which sweeps the prompt out of the chat —
// while the relay is still blocked. Answering afterwards must be refused, not
// recorded against a question nobody is asking.
func TestAnswer_APromptSweptByItsToolCompletingCanNoLongerBeAnswered(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	hook(t, f, runnerID, "claude", engineagents.HookToolPre, map[string]any{
		"tool_use_id": "tool-1", "tool_name": "Bash",
		"tool_input": map[string]any{"command": "touch PROOF"},
	})
	_, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	hook(t, f, runnerID, "claude", engineagents.HookToolPost, map[string]any{
		"tool_use_id": "tool-1", "tool_name": "Bash",
		"tool_input": map[string]any{"command": "touch PROOF"}, "tool_response": "ok",
	})
	require.Empty(t, pendingChoices(t, f, chatID))

	err := f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{"allow"}, "", nil)
	require.ErrorIs(t, err, apperr.ErrConflict)
}

// Answerability is per CHAT as well as per prompt: a relay holding chat A's
// question must never make chat B's look answerable.
func TestAnswer_AnotherChatsRelayDoesNotMakeAPromptAnswerable(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	_, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	assert.Equal(t, []string{choice.ID},
		f.usecase.AnswerableChoiceIDs(chatID, []domain.ActivityChoice{choice}))
	assert.Empty(t,
		f.usecase.AnswerableChoiceIDs("some-other-chat", []domain.ActivityChoice{choice}))
}

// The descriptor is resolved from the LIVE runner's workspace, because the answer
// is about to be printed on that process's own hook. A workspace that cannot be
// read fails the answer rather than guessing at a provider.
func TestAnswer_AnUnreadableWorkspaceFailsTheAnswerRatherThanGuessing(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	_, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	f.ws.err = errors.New("worktree gone")
	err := f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{"allow"}, "", nil)
	require.Error(t, err)
}

// A read that blows up must not be reported as "no such prompt": the two lead a
// client to opposite recoveries.
func TestAnswer_AFailedPendingReadIsAnError(t *testing.T) {
	f, faults := newActivityFaultFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	_, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	faults.choicesErr = errors.New("read blew up")
	err := f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{"allow"}, "", nil)
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperr.ErrConflict)
}

// The decision is RECORDED before the relay is woken. A record that cannot be
// written therefore fails the whole answer, and the relay stays blocked — which
// leaves the CLI's own dialog as the fallback, rather than a CLI that acted on a
// decision the chat has no memory of.
func TestAnswer_AnUnrecordableDecisionIsNotHandedToTheRelay(t *testing.T) {
	f, faults := newActivityWriteFaultFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	deliveryID, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	faults.writeErr = errors.New("log is full")
	err := f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{"allow"}, "", nil)
	require.Error(t, err)

	_, stillWaiting := f.usecase.PendingAnswer(deliveryID)
	assert.True(t, stillWaiting, "the relay must not have been told anything")
}

// Releasing a dead runner's prompts is best-effort: a record that cannot be
// written must not stop the relays being freed, or an orphaned hook would keep
// holding a gate for a process that no longer exists.
func TestAnswer_ADeadRunnerReleasesItsRelaysEvenIfTheRecordCannotBeWritten(t *testing.T) {
	f, faults := newActivityWriteFaultFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	deliveryID, _ := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	printed := await(f, deliveryID)
	faults.writeErr = errors.New("log is full")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	assert.Empty(t, <-printed)
	_, waiting := f.usecase.PendingAnswer(deliveryID)
	assert.False(t, waiting)
}
