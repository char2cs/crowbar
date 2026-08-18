package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
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

// threeQuestionPermission is the payload a user gets by asking claude to "ask me
// 3 questions at the same time": ONE AskUserQuestion call carrying three
// questions, one of them multi-select. It is the payload that stranded a live
// agent — Crowbar recorded the first question only, the human answered it, and
// claude went on saying "still waiting on: your answers to questions 2 & 3".
func threeQuestionPermission() map[string]any {
	return map[string]any{
		"session_id": "s1", "prompt_id": "3q",
		"hook_event_name": "PermissionRequest", "tool_name": "AskUserQuestion",
		"tool_input": map[string]any{
			"questions": []any{
				map[string]any{
					"question": "Which language?", "header": "Language", "multiSelect": false,
					"options": []any{
						map[string]any{"label": "Go"}, map[string]any{"label": "TypeScript"},
					},
				},
				map[string]any{
					"question": "Which databases?", "header": "Storage", "multiSelect": true,
					"options": []any{
						map[string]any{"label": "SQLite"},
						map[string]any{"label": "Postgres"},
						map[string]any{"label": "Redis"},
					},
				},
				map[string]any{
					"question": "Deploy where?", "header": "Target", "multiSelect": false,
					"options": []any{
						map[string]any{"label": "Local"}, map[string]any{"label": "Cloud"},
					},
				},
			},
		},
	}
}

// pick names one option by the question that offers it and its position in that
// question, which is how a human's click reaches the flat list an answer carries.
func pick(t *testing.T, choice domain.ActivityChoice, question, option int) string {
	t.Helper()
	require.Greater(t, len(choice.Questions), question)
	require.Greater(t, len(choice.Questions[question].Options), option)
	return choice.Questions[question].Options[option].ID
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
//
// It does not return until that goroutine is RUNNING, because the order is the
// production order and the test must not invert it: a real relay parks itself the
// instant its hook is delivered, long before any human can answer. Answering
// first would take the slot off the desk before the waiter ever looked for it,
// and the test would be measuring the scheduler rather than the answer channel.
func await(f testFixture, deliveryID string) <-chan string {
	out := make(chan string, 1)
	parked := make(chan struct{})
	go func() {
		close(parked)
		answer, err := f.usecase.AwaitAnswer(f.ctx, deliveryID)
		if err != nil {
			out <- "ERR: " + err.Error()
			return
		}
		out <- string(answer.Stdout)
	}()
	<-parked
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
	require.Len(t, choice.Questions, 1)
	require.Len(t, choice.Questions[0].Options, 2)

	printed := await(f, deliveryID)
	require.NoError(t, f.usecase.AnswerChoice(
		f.ctx, chatID, choice.ID, []string{pick(t, choice, 0, 1)}, "", nil))
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
	require.Len(t, choice.Questions, 1)
	require.True(t, choice.Questions[0].Multi)

	printed := await(f, deliveryID)
	require.NoError(t, f.usecase.AnswerChoice(f.ctx, chatID, choice.ID,
		[]string{pick(t, choice, 0, 0), pick(t, choice, 0, 1)}, "", nil))
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

// DEFECT 4, end to end. A user asked claude to "ask me 3 questions at the same
// time". Claude issued ONE AskUserQuestion carrying three entries; Crowbar
// recorded one prompt with the FIRST of them. The user answered it, the CLI was
// handed an `updatedInput` whose `answers` object had a single key, and claude
// replied "Still waiting on: your answers to questions 2 & 3" — a state no
// surface in Crowbar could ever leave, because there was nothing left to answer.
//
// The whole prompt has to be answerable in ONE call, and the document that
// reaches the CLI has to carry ONE KEY PER QUESTION.
func TestRegression_MultiQuestionAskUserQuestionIsFullyAnswerable(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	deliveryID, choice := blockedPermission(t, f, chatID, runnerID, threeQuestionPermission())

	require.Equal(t, domain.ChoiceKindQuestion, choice.Kind)
	require.Len(t, choice.Questions, 3, "three questions asked is three questions offered")

	printed := await(f, deliveryID)
	require.NoError(t, f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{
		pick(t, choice, 0, 0),                        // Go
		pick(t, choice, 1, 0), pick(t, choice, 1, 2), // SQLite and Redis, multi-select
		pick(t, choice, 2, 1), // Cloud
	}, "", nil))
	f.wait()

	var decoded struct {
		HookSpecificOutput struct {
			Decision struct {
				Behavior     string `json:"behavior"`
				UpdatedInput struct {
					Questions []map[string]any `json:"questions"`
					Answers   map[string]any   `json:"answers"`
				} `json:"updatedInput"`
			} `json:"decision"`
		} `json:"hookSpecificOutput"`
	}
	require.NoError(t, json.Unmarshal([]byte(<-printed), &decoded))
	updated := decoded.HookSpecificOutput.Decision.UpdatedInput
	assert.Equal(t, "allow", decoded.HookSpecificOutput.Decision.Behavior)
	require.Len(t, updated.Questions, 3, "the CLI must get all three questions back")

	require.Len(t, updated.Answers, 3, "one key per question, or the agent waits forever")
	assert.Equal(t, "Go", updated.Answers["Which language?"])
	assert.Equal(t, []any{"SQLite", "Redis"}, updated.Answers["Which databases?"],
		"a multi-select question's value is a LIST, per question")
	assert.Equal(t, "Cloud", updated.Answers["Deploy where?"],
		"a single-select question's value is a bare label in the same document")

	assert.Empty(t, pendingChoices(t, f, chatID), "and the prompt stops pending")
}

// The other half of the same defect: a partial answer must be IMPOSSIBLE, not
// merely discouraged. Sending picks for one of three questions is what handed the
// CLI a partial `updatedInput` in the first place, so it is refused with 400 and
// the relay is left holding the gate for the CLI's own picker.
func TestRegression_APartialAnswerToAMultiQuestionPromptIsRefused(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	deliveryID, choice := blockedPermission(t, f, chatID, runnerID, threeQuestionPermission())

	err := f.usecase.AnswerChoice(
		f.ctx, chatID, choice.ID, []string{pick(t, choice, 0, 0)}, "", nil)
	require.ErrorIs(t, err, apperr.ErrInvalidArgument,
		"one answer to three questions is not an answer")

	err = f.usecase.AnswerChoice(f.ctx, chatID, choice.ID,
		[]string{pick(t, choice, 0, 0), pick(t, choice, 1, 0)}, "", nil)
	require.ErrorIs(t, err, apperr.ErrInvalidArgument, "two of three is not an answer either")

	// Nothing reached the CLI, and the prompt is still there to be answered
	// properly — refusing must not consume the one chance to answer.
	assert.Len(t, pendingChoices(t, f, chatID), 1)
	_, stillWaiting := f.usecase.PendingAnswer(deliveryID)
	assert.True(t, stillWaiting)

	printed := await(f, deliveryID)
	require.NoError(t, f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{
		pick(t, choice, 0, 0), pick(t, choice, 1, 1), pick(t, choice, 2, 0),
	}, "", nil))
	f.wait()
	assert.Contains(t, <-printed, `"Which databases?":["Postgres"]`)
}

// A question that takes ONE answer takes one. Two picks on it are not a smaller
// mistake than none — there is no shape the provider could read them in.
func TestAnswer_RefusesTwoPicksOnASingleSelectQuestion(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	_, choice := blockedPermission(t, f, chatID, runnerID, threeQuestionPermission())

	err := f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{
		pick(t, choice, 0, 0), pick(t, choice, 0, 1), // both languages
		pick(t, choice, 1, 0), pick(t, choice, 2, 0),
	}, "", nil)

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

// A question the provider sent neither text nor header for cannot be keyed at
// all: claude's `answers` object is keyed by the question STRING it sent. Nothing
// is invented for it — an invented key is a key the CLI would not recognise — and
// the rest of the prompt is answered as asked.
func TestAnswer_AQuestionWithNoTextOfItsOwnIsNotGivenAnInventedKey(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	deliveryID, choice := blockedPermission(t, f, chatID, runnerID, map[string]any{
		"session_id": "s1", "prompt_id": "nk", "tool_name": "AskUserQuestion",
		"tool_input": map[string]any{"questions": []any{
			map[string]any{"options": []any{map[string]any{"label": "A"}}},
			map[string]any{
				"question": "Deploy where?",
				"options":  []any{map[string]any{"label": "Cloud"}},
			},
		}},
	})
	require.Len(t, choice.Questions, 2)

	printed := await(f, deliveryID)
	require.NoError(t, f.usecase.AnswerChoice(f.ctx, chatID, choice.ID,
		[]string{pick(t, choice, 0, 0), pick(t, choice, 1, 0)}, "", nil))
	f.wait()

	out := <-printed
	assert.Contains(t, out, `"Deploy where?":"Cloud"`)
	assert.NotContains(t, out, `"":"A"`, "an unnamed question gets no key rather than an empty one")
}

// allowedStdout is the exact document claude 2.1.234 accepts for an allowed Bash
// permission, and what every relay in these ordering tests must end up printing.
const allowedStdout = `{"hookSpecificOutput":{"hookEventName":"PermissionRequest",` +
	`"decision":{"behavior":"allow"}}}`

// THE ORDERING IS THE TEST.
//
// The relay's lifecycle is two round trips from a separate process: it POSTs the
// hook, the daemon acks with "stay alive", and only then does it POST
// /hooks/await. A fast human answers in the gap. The answer used to resolve the
// choice and take the slot off byDelivery at once, so the await that landed a
// moment later found nothing — leaving Crowbar's record saying "answered" while
// the CLI never received a byte and sat on a dialog nobody could clear. That
// split is worse than an unanswerable prompt, because the record lies.
//
// So the verdict outlives the decision, and the late relay still collects it.
func TestRegression_AVerdictDecidedBeforeTheRelayAsksIsStillDelivered(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	deliveryID, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	// Answered with NO waiter parked: the relay's await POST has not landed yet.
	require.NoError(t, f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{"allow"}, "", nil))
	f.wait()

	answer, err := f.usecase.AwaitAnswer(f.ctx, deliveryID)
	require.NoError(t, err)
	assert.JSONEq(t, allowedStdout, string(answer.Stdout),
		"a relay that asked after the decision must still be handed it")

	// And exactly once. A retained verdict is not a mailbox: the second relay on
	// that delivery prints nothing, or the provider runs the gated tool twice.
	second, err := f.usecase.AwaitAnswer(f.ctx, deliveryID)
	require.NoError(t, err)
	assert.Empty(t, second.Stdout, "a claimed verdict is gone")

	all, err := f.usecase.ReadActivity(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, all.Choices, 1)
	assert.Equal(t, domain.ChoiceResolutionAnswered, all.Choices[0].Resolution)
}

// The same window end to end, as a REAL race rather than a sequence: the answer
// and the relay's arrival are released together and nothing here orders them.
//
// What it pins is that the OUTCOME is order-independent — the CLI is told,
// exactly once, whoever won — and that neither ordering deadlocks. The two-sided
// detector is the desk's own race test: a relay parks on a map lookup while an
// answer renders and records, so at this level the relay almost always parks
// first.
func TestAnswer_AnAnswerRacingTheRelaysArrivalReachesItEitherWay(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})

	for i := range 8 {
		payload := permissionPayload()
		payload["tool_input"] = map[string]any{"command": fmt.Sprintf("touch PROOF-%d", i)}
		deliveryID := deliver(t, f, runnerID, "claude", engineagents.HookPermission, payload)
		pending := pendingChoices(t, f, chatID)
		require.Len(t, pending, 1)

		printed := make(chan string, 1)
		start := make(chan struct{})
		var racers sync.WaitGroup
		racers.Add(2)
		go func() {
			defer racers.Done()
			<-start
			answer, err := f.usecase.AwaitAnswer(f.ctx, deliveryID)
			assert.NoError(t, err)
			printed <- string(answer.Stdout)
		}()
		go func() {
			defer racers.Done()
			<-start
			assert.NoError(t, f.usecase.AnswerChoice(
				f.ctx, chatID, pending[0].ID, []string{"allow"}, "", nil))
		}()
		close(start)
		racers.Wait()
		f.wait()

		assert.JSONEq(t, allowedStdout, <-printed, "whoever won the race, the CLI is told")
		require.Empty(t, pendingChoices(t, f, chatID))
	}
}

// A verdict nobody collected is swept when the CLI dies: the relay that would
// have printed it died with its provider, and hooks are spawned DETACHED, so
// nothing else is ever coming for those bytes.
//
// The RECORD is left alone. The prompt really was answered in Crowbar, and
// reporting it as abandoned would rewrite a decision a human made.
func TestAnswer_ADeadRunnerSweepsAVerdictNoRelayCollected(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	deliveryID, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	require.NoError(t, f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{"allow"}, "", nil))
	f.wait()
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	_, waiting := f.usecase.PendingAnswer(deliveryID)
	assert.False(t, waiting, "a dead CLI's uncollected verdict is swept off byDelivery")
	answer, err := f.usecase.AwaitAnswer(f.ctx, deliveryID)
	require.NoError(t, err)
	assert.Empty(t, answer.Stdout)

	all, err := f.usecase.ReadActivity(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, all.Choices, 1)
	assert.Equal(t, domain.ChoiceResolutionAnswered, all.Choices[0].Resolution,
		"the answer a human really gave must not be rewritten as abandoned")
}

// A relay SIGTERMed after the decision but before it collected the verdict is
// reporting its OWN death, not a decision made at the PTY. It takes the
// uncollectable bytes with it and leaves the record alone.
func TestAnswer_AbandonAfterAnAnswerDoesNotRewriteTheRecord(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	deliveryID, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	require.NoError(t, f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{"allow"}, "", nil))
	f.wait()
	require.NoError(t, f.usecase.AbandonAnswer(f.ctx, deliveryID))
	f.wait()

	all, err := f.usecase.ReadActivity(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, all.Choices, 1)
	assert.Equal(t, domain.ChoiceResolutionAnswered, all.Choices[0].Resolution)

	answer, err := f.usecase.AwaitAnswer(f.ctx, deliveryID)
	require.NoError(t, err)
	assert.Empty(t, answer.Stdout, "the relay that reported its own death collects nothing")
}
