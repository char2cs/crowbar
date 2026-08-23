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

func pick(t *testing.T, choice domain.ActivityChoice, question, option int) string {
	t.Helper()
	require.Greater(t, len(choice.Questions), question)
	require.Greater(t, len(choice.Questions[question].Options), option)
	return choice.Questions[question].Options[option].ID
}

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

	err := f.usecase.AnswerChoice(f.ctx, chatID, pending[0].ID, []string{"allow"}, "", nil)
	require.ErrorIs(t, err, apperr.ErrConflict)
}

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

func TestAnswer_RefusesPicksOfDifferentKinds(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	_, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	err := f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{"allow", "deny"}, "", nil)
	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

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

func TestAnswer_AnotherChatsRelayDoesNotMakeAPromptAnswerable(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	_, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	assert.Equal(t, []string{choice.ID},
		f.usecase.AnswerableChoiceIDs(chatID, []domain.ActivityChoice{choice}))
	assert.Empty(t,
		f.usecase.AnswerableChoiceIDs("some-other-chat", []domain.ActivityChoice{choice}))
}

func TestAnswer_AnUnreadableWorkspaceFailsTheAnswerRatherThanGuessing(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	_, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	f.ws.err = errors.New("worktree gone")
	err := f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{"allow"}, "", nil)
	require.Error(t, err)
}

func TestAnswer_AFailedPendingReadIsAnError(t *testing.T) {
	f, faults := newActivityFaultFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	_, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	faults.choicesErr = errors.New("read blew up")
	err := f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{"allow"}, "", nil)
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperr.ErrConflict)
}

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

func TestRegression_MultiQuestionAskUserQuestionIsFullyAnswerable(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	deliveryID, choice := blockedPermission(t, f, chatID, runnerID, threeQuestionPermission())

	require.Equal(t, domain.ChoiceKindQuestion, choice.Kind)
	require.Len(t, choice.Questions, 3, "three questions asked is three questions offered")

	printed := await(f, deliveryID)
	require.NoError(t, f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{
		pick(t, choice, 0, 0),
		pick(t, choice, 1, 0), pick(t, choice, 1, 2),
		pick(t, choice, 2, 1),
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

func TestAnswer_RefusesTwoPicksOnASingleSelectQuestion(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	_, choice := blockedPermission(t, f, chatID, runnerID, threeQuestionPermission())

	err := f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{
		pick(t, choice, 0, 0), pick(t, choice, 0, 1),
		pick(t, choice, 1, 0), pick(t, choice, 2, 0),
	}, "", nil)

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

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

const allowedStdout = `{"hookSpecificOutput":{"hookEventName":"PermissionRequest",` +
	`"decision":{"behavior":"allow"}}}`

func TestRegression_AVerdictDecidedBeforeTheRelayAsksIsStillDelivered(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	deliveryID, choice := blockedPermission(t, f, chatID, runnerID, permissionPayload())

	require.NoError(t, f.usecase.AnswerChoice(f.ctx, chatID, choice.ID, []string{"allow"}, "", nil))
	f.wait()

	answer, err := f.usecase.AwaitAnswer(f.ctx, deliveryID)
	require.NoError(t, err)
	assert.JSONEq(t, allowedStdout, string(answer.Stdout),
		"a relay that asked after the decision must still be handed it")

	second, err := f.usecase.AwaitAnswer(f.ctx, deliveryID)
	require.NoError(t, err)
	assert.Empty(t, second.Stdout, "a claimed verdict is gone")

	all, err := f.usecase.ReadActivity(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, all.Choices, 1)
	assert.Equal(t, domain.ChoiceResolutionAnswered, all.Choices[0].Resolution)
}

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
