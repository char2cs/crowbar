package agent_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func permissionPayload() map[string]any {
	return map[string]any{
		"session_id": "s1", "prompt_id": "81899da5", "permission_mode": "default",
		"hook_event_name": "PermissionRequest", "tool_name": "Bash",
		"tool_input": map[string]any{
			"command": "touch PROOF", "description": "Create proof control file",
		},
		"permission_suggestions": []any{
			map[string]any{
				"type": "addDirectories", "directories": []any{"/proof"},
				"destination": "session",
			},
			map[string]any{"type": "setMode", "mode": "acceptEdits", "destination": "session"},
		},
	}
}

func pendingChoices(t *testing.T, f testFixture, chatID string) []domain.ActivityChoice {
	t.Helper()
	got, err := f.usecase.ReadPendingChoices(f.ctx, chatID)
	require.NoError(t, err)
	return got
}

func TestObservation_APermissionIsRecordedAsAPendingChoice(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	hook(t, f, runnerID, "claude", engineagents.HookPermission, permissionPayload())

	got := pendingChoices(t, f, chatID)
	require.Len(t, got, 1)
	assert.Equal(t, domain.ChoiceKindPermission, got[0].Kind)
	assert.Equal(t, "81899da5", got[0].PromptID)
	assert.Equal(t, "Bash", got[0].ToolName)
	assert.True(t, got[0].Pending())
	require.Len(t, got[0].Options, 4, "allow, deny, and both of claude's suggestions")
	assert.Equal(t, domain.ChoiceOptionAllow, got[0].Options[0].Kind)
	assert.Equal(t, domain.ChoiceOptionDeny, got[0].Options[1].Kind)
	assert.Equal(t, "Allow this directory from now on", got[0].Options[2].Label)
	assert.Equal(t, "Switch to a more permissive mode", got[0].Options[3].Label)
	assert.NotEmpty(t, got[0].ID, "a future answer has to be able to name this record")
}

func TestRegression_NoPromptEverCarriesARawProviderTypeNameAsALabel(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	payload := permissionPayload()
	payload["permission_suggestions"] = []any{
		map[string]any{"type": "addRules", "destination": "session"},
		map[string]any{"type": "aTypeNobodyHasCaptured", "destination": "session"},
	}
	hook(t, f, runnerID, "claude", engineagents.HookPermission, payload)

	got := pendingChoices(t, f, chatID)
	require.Len(t, got, 1)
	require.Len(t, got[0].Options, 4)
	for _, option := range got[0].Options {
		assert.NotEqual(t, "addRules", option.Label)
		assert.NotEqual(t, "aTypeNobodyHasCaptured", option.Label)
		assert.NotContains(t, option.Label, "addRules")
	}
	assert.Equal(t, "Add a permanent rule for this", got[0].Options[2].Label)
	assert.Equal(t, "A broader permission than this one", got[0].Options[3].Label)
}

func TestObservation_APermissionAdoptsTheInFlightCallItGates(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	hook(t, f, runnerID, "claude", engineagents.HookToolPre, map[string]any{
		"tool_use_id": "tool-1", "tool_name": "Bash",
		"tool_input": map[string]any{"command": "touch PROOF"},
	})
	hook(t, f, runnerID, "claude", engineagents.HookPermission, permissionPayload())

	got := pendingChoices(t, f, chatID)
	require.Len(t, got, 1)
	assert.Equal(t, "tool-1", got[0].ToolID)
}

func TestObservation_APendingChoiceClearsWhenTheGatedToolProceeds(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	hook(t, f, runnerID, "claude", engineagents.HookToolPre,
		map[string]any{"tool_use_id": "tool-1", "tool_name": "Bash"})
	hook(t, f, runnerID, "claude", engineagents.HookPermission, permissionPayload())
	require.Len(t, pendingChoices(t, f, chatID), 1)

	hook(t, f, runnerID, "claude", engineagents.HookToolPost, map[string]any{
		"tool_use_id": "tool-1", "tool_name": "Bash", "tool_response": "ok",
	})

	assert.Empty(t, pendingChoices(t, f, chatID),
		"a prompt answered outside Crowbar must still stop being pending")
	all, err := f.usecase.ReadActivity(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, all.Choices, 1)
	assert.Equal(t, domain.ChoiceResolutionProceeded, all.Choices[0].Resolution)
}

func TestObservation_APendingChoiceClearsWhenTheGatedToolFails(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	hook(t, f, runnerID, "claude", engineagents.HookToolPre,
		map[string]any{"tool_use_id": "tool-1", "tool_name": "Bash"})
	hook(t, f, runnerID, "claude", engineagents.HookPermission, permissionPayload())

	hook(t, f, runnerID, "claude", engineagents.HookToolFail, map[string]any{
		"tool_use_id": "tool-1", "tool_name": "Bash",
		"error": "exit status 1", "is_interrupt": false, "duration_ms": 42,
	})

	assert.Empty(t, pendingChoices(t, f, chatID))
}

func TestObservation_APendingChoiceDoesNotSurviveItsTurn(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	hook(t, f, runnerID, "claude", engineagents.HookPermission, permissionPayload())
	require.Len(t, pendingChoices(t, f, chatID), 1)

	turn(t, f, runnerID, "claude", "I gave up on that")

	assert.Empty(t, pendingChoices(t, f, chatID))
	all, err := f.usecase.ReadActivity(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, all.Choices, 1)
	assert.Equal(t, domain.ChoiceResolutionAbandoned, all.Choices[0].Resolution)
}

func TestObservation_APermissionWithNoTurnOpenIsNotPending(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookPermission, permissionPayload())

	assert.Empty(t, pendingChoices(t, f, chatID))
	all, err := f.usecase.ReadActivity(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, all.Choices, 1, "it is still recorded — just not as something to answer")
	assert.False(t, all.Choices[0].Pending())
}

func TestObservation_AskUserQuestionIsRecordedWithItsLabelledOptions(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	hook(t, f, runnerID, "claude", engineagents.HookPermission, map[string]any{
		"session_id": "s1", "prompt_id": "q1", "tool_name": "AskUserQuestion",
		"tool_input": map[string]any{"questions": []any{map[string]any{
			"question": "Do you prefer option A or option B?",
			"header":   "Pick",
			"options": []any{
				map[string]any{"label": "A", "description": "Option A"},
				map[string]any{"label": "B", "description": "Option B"},
			},
			"multiSelect": false,
		}}},
	})

	got := pendingChoices(t, f, chatID)
	require.Len(t, got, 1)
	assert.Equal(t, domain.ChoiceKindQuestion, got[0].Kind)
	assert.Equal(t, "Pick", got[0].Title)
	assert.Equal(t, "Do you prefer option A or option B?", got[0].Question)

	require.Len(t, got[0].Questions, 1)
	question := got[0].Questions[0]
	assert.Equal(t, "Pick", question.Title)
	assert.Equal(t, "Do you prefer option A or option B?", question.Text)
	assert.False(t, question.Multi)
	require.Len(t, question.Options, 2)
	assert.Equal(t, domain.ChoiceOptionAnswer, question.Options[0].Kind)
	assert.Equal(t, "A", question.Options[0].Label)
	assert.Equal(t, "B", question.Options[1].Label)
}

func TestObservation_AMultiQuestionAskIsRecordedWithEveryQuestion(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	hook(t, f, runnerID, "claude", engineagents.HookPermission, threeQuestionPermission())

	got := pendingChoices(t, f, chatID)
	require.Len(t, got, 1)
	require.Len(t, got[0].Questions, 3, "three questions asked is three questions stored")
	assert.Equal(t, "Which language?", got[0].Questions[0].Text)
	assert.True(t, got[0].Questions[1].Multi, "multiSelect is per question")
	assert.Equal(t, "Deploy where?", got[0].Questions[2].Text)
	assert.Empty(t, got[0].Options, "a question's options live on the question")
}

func TestObservation_AnElicitationIsRecordedAsAnInterruptionAndAPrompt(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	hook(t, f, runnerID, "claude", engineagents.HookElicitation, map[string]any{
		"hook_event_name": "Elicitation", "mcp_server_name": "spike",
		"message": "do you prefer A or B?", "mode": "form",
		"requested_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"choice": map[string]any{"type": "string", "enum": []any{"A", "B"}},
			},
			"required": []any{"choice"},
		},
	})

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1)
	assert.Equal(t, engineagents.InterruptElicitation, ints[0].Kind)

	got := pendingChoices(t, f, chatID)
	require.Len(t, got, 1)
	assert.Equal(t, domain.ChoiceKindElicitation, got[0].Kind)
	assert.Equal(t, "spike", got[0].Title)
	assert.Equal(t, "do you prefer A or B?", got[0].Question)
	assert.Equal(t, "form", got[0].Mode)
	assert.Contains(t, got[0].Schema, `"enum":["A","B"]`)
}

func TestObservation_ACodexChatObservesNeitherElicitationNorToolFailure(t *testing.T) {
	testCases := []struct {
		name    string
		kind    string
		payload map[string]any
	}{
		{
			name: "elicitation", kind: engineagents.HookElicitation,
			payload: map[string]any{"mcp_server_name": "spike", "message": "A or B?"},
		},
		{
			name: "tool failure", kind: engineagents.HookToolFail,
			payload: map[string]any{"tool_use_id": "t1", "tool_name": "shell", "error": "boom"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			chatID, runnerID := f.spawn(t, "codex")
			hook(t, f, runnerID, "codex", engineagents.HookUserPrompt,
				map[string]any{"prompt": "go"})

			err := f.usecase.IngestHook(f.ctx, runnerID, "codex", tc.kind, mustJSON(t, tc.payload))
			f.wait()

			require.NoError(t, err, "an unmapped kind is dropped, never failed")
			assert.Empty(t, pendingChoices(t, f, chatID))
			calls, listErr := f.activity.ToolCalls(f.ctx, chatID, 0, 0)
			require.NoError(t, listErr)
			assert.Empty(t, calls)
		})
	}
}

func TestObservation_ACodexPermissionReportsAllowAndDenyAndNothingInvented(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")

	hook(t, f, runnerID, "codex", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	hook(t, f, runnerID, "codex", engineagents.HookPermission, map[string]any{
		"session_id": "s1", "tool_name": "shell",
		"tool_input": map[string]any{"command": "rm -rf /"},
	})

	got := pendingChoices(t, f, chatID)
	require.Len(t, got, 1)
	assert.Equal(t, domain.ChoiceKindPermission, got[0].Kind)
	assert.Equal(t, "shell", got[0].ToolName)
	assert.Empty(t, got[0].PromptID, "codex is not claimed to send a prompt id")
	require.Len(t, got[0].Options, 2)
	assert.Equal(t, domain.ChoiceOptionAllow, got[0].Options[0].Kind)
	assert.Equal(t, domain.ChoiceOptionDeny, got[0].Options[1].Kind)
}

func TestRegression_AFailedToolIsCompletedNotLeftRunning(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	hook(t, f, runnerID, "claude", engineagents.HookToolPre, map[string]any{
		"tool_use_id": "tool-1", "tool_name": "Bash",
		"tool_input": map[string]any{"command": "false"},
	})

	running, err := f.activity.ToolCalls(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, running, 1)
	require.Equal(t, domain.ToolStatusRunning, running[0].Status)

	hook(t, f, runnerID, "claude", engineagents.HookToolFail, map[string]any{
		"tool_use_id": "tool-1", "tool_name": "Bash",
		"tool_input": map[string]any{"command": "false"},
		"error":      "exit status 1", "is_interrupt": false, "duration_ms": 42,
	})

	after, err := f.activity.ToolCalls(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, after, 1)
	assert.Equal(t, domain.ToolStatusError, after[0].Status,
		"a failed tool is failed, not running and not abandoned")
	require.NotNil(t, after[0].EndedAt)
	assert.Equal(t, "exit status 1", after[0].Error)
	assert.Equal(t, 42, after[0].DurationMS)

	payload, err := f.usecase.ReadToolPayload(f.ctx, chatID, "tool-1", "result")
	require.NoError(t, err)
	assert.Equal(t, "exit status 1", string(payload))
}

func TestRegression_AnInterruptedToolIsFailedNotAbandoned(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	hook(t, f, runnerID, "claude", engineagents.HookToolPre,
		map[string]any{"tool_use_id": "tool-1", "tool_name": "Bash"})
	hook(t, f, runnerID, "claude", engineagents.HookToolFail, map[string]any{
		"tool_use_id": "tool-1", "tool_name": "Bash",
		"error": "interrupted by user", "is_interrupt": true, "duration_ms": 9,
	})
	turn(t, f, runnerID, "claude", "stopped")

	after, err := f.activity.ToolCalls(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, after, 1)
	assert.Equal(t, domain.ToolStatusError, after[0].Status)
	assert.NotEqual(t, domain.ToolStatusAbandoned, after[0].Status,
		"the turn-close sweep must find nothing left to abandon")
}

func TestObservation_AFailedChoiceWriteDoesNotFailTheHook(t *testing.T) {
	f, faults := newActivityWriteFaultFixture(t)
	_, runnerID := f.spawn(t, "claude")
	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	faults.writeErr = errors.New("record unavailable")

	err := f.usecase.IngestHook(f.ctx, runnerID, "claude", engineagents.HookPermission,
		mustJSON(t, permissionPayload()))
	f.wait()

	assert.NoError(t, err)
}

func TestReadPendingChoices_RefusesAChatThatDoesNotExist(t *testing.T) {
	f := newFixture(t)

	_, err := f.usecase.ReadPendingChoices(f.ctx, "no-such-chat")

	assert.Error(t, err)
}

func TestReadPendingChoices_PropagatesAReadModelFailure(t *testing.T) {
	f, activity := newActivityFaultFixture(t)
	chatID, _ := f.spawn(t, "claude")
	activity.choicesErr = errors.New("read model unavailable")

	_, err := f.usecase.ReadPendingChoices(f.ctx, chatID)

	assert.Error(t, err)
}

func TestReadActivity_PropagatesAPromptReadFailure(t *testing.T) {
	f, activity := newActivityFaultFixture(t)
	chatID, _ := f.spawn(t, "claude")
	activity.choicesErr = errors.New("read model unavailable")

	_, err := f.usecase.ReadActivity(f.ctx, chatID, 0, 0)

	assert.Error(t, err)
}
