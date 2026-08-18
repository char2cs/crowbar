package agent_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// permissionPayload is the PermissionRequest captured live from claude 2.1.234 on
// 2026-08-17. Everything but tool_name used to be discarded on the way in, so a
// blocked agent rendered as "waiting on Bash" with no way to see what Bash was
// about to do.
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

// DEFECT 5, end to end through the shipped descriptor. `addRules` is claude's own
// machine name for a broader grant, and reading it onto an option put that string
// in the chat as something a person could press — spelled in a vocabulary only the
// CLI's source uses, on the one path the backend refuses with a 400.
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

// The permission carries no tool_use_id, so the in-flight PreToolUse of the same
// name is what says which call is being gated.
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

// THE defensive case. A permission is almost always answered at the PTY, by a
// human typing into the vendor CLI, and nothing reports that happening — so the
// gated work proceeding has to clear the prompt, or the UI is stranded on a
// question nobody is asking any more.
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

// A tool that FAILS answered its permission too — the question was answered, the
// work simply went badly afterwards.
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

// The turn boundary is the backstop, for the prompts nothing else can clear.
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

// The shipped precedent, which this must not regress: claude fires a permission
// notification about a minute AFTER a turn ends. An agent that is not running is
// not blocked, and a pending prompt over an idle agent is a banner nothing clears.
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

// AskUserQuestion is a TOOL on claude, so its question arrives inside the
// permission's own tool input rather than as an event of its own.
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

	// One question is a LIST OF ONE. There is no second shape for the one-question
	// case, so nothing downstream branches on how many were asked.
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

// The record has to survive the round trip through the event log and the
// projection, or a prompt would be answerable only in the instant it arrived.
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

// Elicitation is a hook event of its own — an MCP server asking through the CLI.
// It is also the only thing that produces the elicitation interruption kind,
// which was a declared constant nothing wrote until now.
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

// codex declares neither Elicitation nor PostToolUseFailure. An unmapped kind
// must degrade to never being reported — never to a wrong value — and must not
// fail the hook, because a hook must never break the vendor CLI's turn.
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

// codex DOES declare PermissionRequest, and its descriptor maps the tool
// vocabulary its own binary declares — so a codex chat reports a real prompt with
// allow and deny, and none of claude's suggestion machinery.
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

// PostToolUseFailure fires INSTEAD OF PostToolUse (measured against claude 2.1.234
// on 2026-08-17), so a failing tool was never completed in the record: it stayed
// in flight until the turn-close sweep abandoned it, and "the Bash call failed"
// rendered as "the Bash call is still running" for the rest of the turn.
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

	// The failure arrives, and NO PostToolUse ever does.
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

	// And the failure text is addressable, so a UI can show the whole of it.
	payload, err := f.usecase.ReadToolPayload(f.ctx, chatID, "tool-1", "result")
	require.NoError(t, err)
	assert.Equal(t, "exit status 1", string(payload))
}

// A tool that is INTERRUPTED reports the same way, and must not be conflated with
// one Crowbar swept because a CLI died.
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

// A write that fails must not break the vendor CLI's turn: the prompt is a gap in
// a timeline, and failing the hook would be a broken agent.
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

// A chat that does not exist is a bad request, not an empty prompt list: a client
// polling a deleted chat has to learn that rather than see "waiting on nothing"
// forever.
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

// The timeline carries the prompts beside the tool calls, so a prompt read that
// fails must fail the whole read rather than silently drop the one section a
// blocked user is looking for.
func TestReadActivity_PropagatesAPromptReadFailure(t *testing.T) {
	f, activity := newActivityFaultFixture(t)
	chatID, _ := f.spawn(t, "claude")
	activity.choicesErr = errors.New("read model unavailable")

	_, err := f.usecase.ReadActivity(f.ctx, chatID, 0, 0)

	assert.Error(t, err)
}
