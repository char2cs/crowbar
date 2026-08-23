package agent_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/chatlog"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func hook(t *testing.T, f testFixture, runnerID, provider, kind string, payload map[string]any) {
	t.Helper()
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, provider, kind, mustJSON(t, payload)))
	f.wait()
}

func TestObservation_ToolActivityIsRecordedWithItsPayloads(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt,
		map[string]any{"prompt": "edit the file"})
	hook(t, f, runnerID, "claude", engineagents.HookToolPre, map[string]any{
		"tool_use_id": "tool-1", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": "a.go", "old_string": "x"},
	})
	hook(t, f, runnerID, "claude", engineagents.HookToolPost, map[string]any{
		"tool_use_id": "tool-1", "tool_name": "Edit",
		"tool_input":    map[string]any{"file_path": "a.go"},
		"tool_response": "applied", "duration_ms": 37,
	})

	calls, err := f.activity.ToolCalls(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.Equal(t, "Edit", calls[0].Name)
	assert.Equal(t, "a.go", calls[0].Target)
	assert.Equal(t, domain.ToolStatusOK, calls[0].Status)
	assert.Equal(t, 37, calls[0].DurationMS)

	require.NotEmpty(t, calls[0].RequestRef)
	request, err := f.activity.Payload(f.ctx, calls[0].RequestRef)
	require.NoError(t, err)
	assert.Contains(t, string(request), "old_string")
	result, err := f.activity.Payload(f.ctx, calls[0].ResultRef)
	require.NoError(t, err)
	assert.Equal(t, "applied", string(result))
}

func TestObservation_ToolCallsAttachToTheTurnThePromptOpened(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	hook(t, f, runnerID, "claude", engineagents.HookToolPre,
		map[string]any{"tool_use_id": "t1", "tool_name": "Bash"})
	hook(t, f, runnerID, "claude", engineagents.HookToolPost,
		map[string]any{"tool_use_id": "t1", "tool_name": "Bash"})
	turn(t, f, runnerID, "claude", "done")

	calls, err := f.activity.ToolCalls(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	require.NotEmpty(t, calls[0].TurnID)

	turns, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	var assistant *domain.ActivityTurn
	for i := range turns {
		if turns[i].Role == domain.TurnRoleAssistant {
			assistant = &turns[i]
		}
	}
	require.NotNil(t, assistant)
	assert.Equal(t, assistant.ID, calls[0].TurnID,
		"a tool call must be attributable to the reply it produced")
}

func TestObservation_SubagentsAreRecorded(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookSubagentPre,
		map[string]any{"agent_id": "a1", "agent_type": "explore"})
	hook(t, f, runnerID, "claude", engineagents.HookSubagentPost,
		map[string]any{"agent_id": "a1", "agent_type": "explore"})

	subs, err := f.activity.Subagents(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	assert.Equal(t, "explore", subs[0].AgentType)
	assert.NotNil(t, subs[0].EndedAt)
}

func TestObservation_AnonymousSubagentStopsDoNotCollide(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookSubagentPost, map[string]any{"agent_type": ""})
	hook(t, f, runnerID, "claude", engineagents.HookSubagentPost, map[string]any{"agent_type": ""})

	subs, err := f.activity.Subagents(f.ctx, chatID)
	require.NoError(t, err)
	assert.Len(t, subs, 2)
}

func TestObservation_InterruptionsAreRecordedForEachKind(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookNotification,
		map[string]any{"message": "Claude needs your permission"})
	hook(t, f, runnerID, "claude", engineagents.HookPermission,
		map[string]any{"tool_name": "Bash"})

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 2)
	kinds := []string{ints[0].Kind, ints[1].Kind}
	assert.Contains(t, kinds, engineagents.InterruptNotification)
	assert.Contains(t, kinds, engineagents.InterruptPermission)
}

func TestObservation_ACompactionOpensAndResolvesTheSameRecord(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookCompactPre, map[string]any{"trigger": "auto"})
	hook(t, f, runnerID, "claude", engineagents.HookCompactPost, map[string]any{"trigger": "auto"})

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1)
	assert.Equal(t, engineagents.InterruptCompaction, ints[0].Kind)
	assert.NotNil(t, ints[0].ResolvedAt)
}

func TestObservation_SessionEndRecordsNothingOfItsOwn(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "hi"})

	before, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)

	hook(t, f, runnerID, "claude", engineagents.HookSessionEnd, map[string]any{"reason": "exit"})

	after, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	assert.Len(t, after, len(before))
}

func TestObservation_AnUndeclaredEventIsDroppedNotFailed(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")

	err := f.usecase.IngestHook(f.ctx, runnerID, "codex", engineagents.HookNotification,
		mustJSON(t, map[string]any{"transcript_path": "/rollouts/x", "message": "hi"}))
	f.wait()

	require.NoError(t, err)
	ints, listErr := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, listErr)
	assert.Empty(t, ints)
}

func TestTelemetry_IsHeldPerChatAndReplacedByTheNextReport(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	_, ok := f.usecase.Telemetry(chatID)
	assert.False(t, ok, "a chat with no report has no gauge")

	hook(t, f, runnerID, "claude", engineagents.HookTelemetry, map[string]any{
		"context_window": map[string]any{"context_window_size": 200000, "used_percentage": 19},
		"model":          map[string]any{"id": "m", "display_name": "M"},
	})

	got, ok := f.usecase.Telemetry(chatID)
	require.True(t, ok)
	require.NotNil(t, got.Context)
	assert.Equal(t, 200000, *got.Context.CapacityTokens)
	assert.InDelta(t, 19, *got.Context.UsedPercent, 0.001)

	hook(t, f, runnerID, "claude", engineagents.HookTelemetry, map[string]any{
		"context_window": map[string]any{"context_window_size": 200000, "used_percentage": 42},
	})

	got, ok = f.usecase.Telemetry(chatID)
	require.True(t, ok)
	assert.InDelta(t, 42, *got.Context.UsedPercent, 0.001)
}

func TestTelemetry_AnEmptyReportDoesNotOverwriteTheLastOne(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookTelemetry, map[string]any{
		"context_window": map[string]any{"used_percentage": 19},
	})
	hook(t, f, runnerID, "claude", engineagents.HookTelemetry, map[string]any{
		"context_window": nil, "cost": nil, "model": nil,
	})

	got, ok := f.usecase.Telemetry(chatID)
	require.True(t, ok)
	require.NotNil(t, got.Context)
	assert.InDelta(t, 19, *got.Context.UsedPercent, 0.001)
}

func TestTelemetry_IsNotRecordedAsATurn(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookTelemetry, map[string]any{
		"context_window": map[string]any{"used_percentage": 19},
	})

	turns, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, turns)
}

func TestTelemetry_ForAProviderWithNoChannelIsSilentlyIgnored(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")

	err := f.usecase.IngestHook(f.ctx, runnerID, "codex", engineagents.HookTelemetry,
		mustJSON(t, map[string]any{"context_window": map[string]any{"used_percentage": 19}}))
	f.wait()

	require.NoError(t, err, "a hook must never break the vendor CLI's turn")
	_, ok := f.usecase.Telemetry(chatID)
	assert.False(t, ok)
}

func TestTelemetry_IsDroppedWhenTheChatIsPurged(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	hook(t, f, runnerID, "claude", engineagents.HookTelemetry, map[string]any{
		"context_window": map[string]any{"used_percentage": 19},
	})
	_, ok := f.usecase.Telemetry(chatID)
	require.True(t, ok)

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	_, ok = f.usecase.Telemetry(chatID)
	assert.False(t, ok, "a report must not outlive the chat it describes")
}

func TestReadMessages_PagesForwardAndBackward(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	for _, text := range []string{"m0", "m1", "m2", "m3", "m4"} {
		turn(t, f, runnerID, "claude", text)
	}

	all, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 50)
	require.NoError(t, err)
	require.Len(t, all.Items, 5)
	assert.Equal(t, "m0", all.Items[0].Text)
	assert.Equal(t, "m4", all.Items[4].Text)
	assert.False(t, all.HasMore)

	newest, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 2)
	require.NoError(t, err)
	require.Len(t, newest.Items, 2)
	assert.Equal(t, "m3", newest.Items[0].Text)
	assert.True(t, newest.HasMore, "older messages remain")

	older, err := f.usecase.ReadMessages(f.ctx, chatID, 0, newest.OldestCursor, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"m1", "m2"}, textsOfPage(older.Items))

	forward, err := f.usecase.ReadMessages(f.ctx, chatID, all.Items[1].Sequence, 0, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"m2", "m3"}, textsOfPage(forward.Items))
	assert.True(t, forward.HasMore, "newer messages remain")
}

func TestReadMessages_RefusesAnAmbiguousOrOversizedRequest(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.ReadMessages(f.ctx, chatID, 5, 5, 10)
	assert.Error(t, err, "after and before are mutually exclusive")

	_, err = f.usecase.ReadMessages(f.ctx, chatID, -1, 0, 10)
	assert.Error(t, err)

	_, err = f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 100000)
	assert.Error(t, err)
}

func textsOfPage(items []chatlog.Message) []string {
	out := make([]string, 0, len(items))
	for _, m := range items {
		out = append(out, m.Text)
	}
	return out
}

func TestObservation_AnonymousToolCallsDoNotCollide(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookToolPre, map[string]any{"tool_name": "Bash"})
	hook(t, f, runnerID, "claude", engineagents.HookToolPre, map[string]any{"tool_name": "Bash"})

	calls, err := f.activity.ToolCalls(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	assert.Len(t, calls, 2)
}

func TestObservation_AToolCompletionWithNoStatusReadsAsOK(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookToolPre,
		map[string]any{"tool_use_id": "t1", "tool_name": "Read"})
	hook(t, f, runnerID, "claude", engineagents.HookToolPost,
		map[string]any{"tool_use_id": "t1", "tool_name": "Read"})

	calls, err := f.activity.ToolCalls(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.Equal(t, domain.ToolStatusOK, calls[0].Status)
}

func TestObservation_ARecordFailureNeverBreaksTheHook(t *testing.T) {
	f, activity := newActivityWriteFaultFixture(t)
	_, runnerID := f.spawn(t, "claude")
	activity.writeErr = errors.New("record unavailable")

	for _, tc := range []struct {
		kind    string
		payload map[string]any
	}{
		{engineagents.HookToolPre, map[string]any{"tool_use_id": "t1", "tool_name": "Bash"}},
		{engineagents.HookToolPost, map[string]any{"tool_use_id": "t1", "tool_name": "Bash"}},
		{engineagents.HookSubagentPre, map[string]any{"agent_id": "a1"}},
		{engineagents.HookSubagentPost, map[string]any{"agent_id": "a1"}},
		{engineagents.HookNotification, map[string]any{"message": "blocked"}},
		{engineagents.HookPermission, map[string]any{"tool_name": "Bash"}},
		{engineagents.HookCompactPre, map[string]any{"trigger": "auto"}},
		{engineagents.HookCompactPost, map[string]any{"trigger": "auto"}},
	} {
		err := f.usecase.IngestHook(f.ctx, runnerID, "claude", tc.kind, mustJSON(t, tc.payload))
		assert.NoError(t, err, tc.kind)
	}
}

func TestObservation_AHookFromARunnerPlacedNowhereIsDropped(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	err := f.usecase.IngestHook(f.ctx, runnerID, "claude", engineagents.HookToolPre,
		mustJSON(t, map[string]any{"tool_use_id": "t1", "tool_name": "Bash"}))

	assert.NoError(t, err)
}

func TestRegression_ADeadCLIDoesNotLeaveItsToolsRunningForever(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	hook(t, f, runnerID, "claude", engineagents.HookToolPre,
		map[string]any{"tool_use_id": "t1", "tool_name": "Bash"})

	running, err := f.activity.ToolCalls(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, running, 1)
	require.Equal(t, domain.ToolStatusRunning, running[0].Status)

	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	after, err := f.activity.ToolCalls(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, after, 1)
	assert.Equal(t, domain.ToolStatusAbandoned, after[0].Status)
	assert.NotNil(t, after[0].EndedAt)
}

func TestRegression_AfterADeadCLIANewTurnOwnsItsOwnActivity(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "first"})
	hook(t, f, runnerID, "claude", engineagents.HookToolPre,
		map[string]any{"tool_use_id": "t1", "tool_name": "Bash"})
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	revivedRunnerID, err := f.usecase.StartRunner(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()
	hook(t, f, revivedRunnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "second"})
	hook(t, f, revivedRunnerID, "claude", engineagents.HookToolPre,
		map[string]any{"tool_use_id": "t2", "tool_name": "Read"})
	turn(t, f, revivedRunnerID, "claude", "the second reply")

	calls, err := f.activity.ToolCalls(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, calls, 2)
	assert.NotEqual(t, calls[0].TurnID, calls[1].TurnID,
		"a new turn must not inherit the activity of the one a dead CLI abandoned")
}

func TestReadActivity_ReturnsWhatTheAgentDid(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	hook(t, f, runnerID, "claude", engineagents.HookToolPre,
		map[string]any{"tool_use_id": "t1", "tool_name": "Bash", "tool_input": map[string]any{"command": "ls"}})
	hook(t, f, runnerID, "claude", engineagents.HookSubagentPre, map[string]any{"agent_id": "a1"})
	hook(t, f, runnerID, "claude", engineagents.HookPermission, map[string]any{"tool_name": "Bash"})

	got, err := f.usecase.ReadActivity(f.ctx, chatID, 0, 0)

	require.NoError(t, err)
	require.Len(t, got.ToolCalls, 1)
	assert.Equal(t, "Bash", got.ToolCalls[0].Name)
	assert.Len(t, got.Subagents, 1)
	assert.Len(t, got.Interruptions, 1)
}

func TestReadActivity_PagesToolCallsFromACursor(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	for _, id := range []string{"t1", "t2", "t3"} {
		hook(t, f, runnerID, "claude", engineagents.HookToolPre,
			map[string]any{"tool_use_id": id, "tool_name": id})
	}

	all, err := f.usecase.ReadActivity(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, all.ToolCalls, 3)

	page, err := f.usecase.ReadActivity(f.ctx, chatID, all.ToolCalls[0].Seq, 1)
	require.NoError(t, err)
	require.Len(t, page.ToolCalls, 1)
	assert.Equal(t, "t2", page.ToolCalls[0].Name)
}

func TestReadActivity_RefusesAChatThatDoesNotExist(t *testing.T) {
	f := newFixture(t)

	_, err := f.usecase.ReadActivity(f.ctx, "no-such-chat", 0, 0)

	require.Error(t, err)
}

func TestReadToolPayload_ResolvesBothSides(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	hook(t, f, runnerID, "claude", engineagents.HookToolPre, map[string]any{
		"tool_use_id": "t1", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": "a.go"},
	})
	hook(t, f, runnerID, "claude", engineagents.HookToolPost, map[string]any{
		"tool_use_id": "t1", "tool_name": "Edit", "tool_response": "applied",
	})

	request, err := f.usecase.ReadToolPayload(f.ctx, chatID, "t1", "request")
	require.NoError(t, err)
	assert.Contains(t, string(request), "a.go")

	result, err := f.usecase.ReadToolPayload(f.ctx, chatID, "t1", "result")
	require.NoError(t, err)
	assert.Equal(t, "applied", string(result))
}

func TestReadToolPayload_IsScopedToTheChatThatOwnsTheToolCall(t *testing.T) {
	f := newFixture(t)
	ownerChat, ownerRunner := f.spawn(t, "claude")
	hook(t, f, ownerRunner, "claude", engineagents.HookToolPre, map[string]any{
		"tool_use_id": "t1", "tool_name": "Read",
		"tool_input": map[string]any{"file_path": "secret.go"},
	})
	otherChat, _ := f.spawn(t, "claude")

	_, err := f.usecase.ReadToolPayload(f.ctx, otherChat, "t1", "request")
	assert.ErrorIs(t, err, agentactivity.ErrNotFound)

	_, err = f.usecase.ReadToolPayload(f.ctx, ownerChat, "t1", "request")
	assert.NoError(t, err)
}

func TestReadToolPayload_MissingToolOrSideIsNotFound(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	hook(t, f, runnerID, "claude", engineagents.HookToolPre,
		map[string]any{"tool_use_id": "t1", "tool_name": "Bash"})

	_, err := f.usecase.ReadToolPayload(f.ctx, chatID, "no-such-tool", "request")
	assert.ErrorIs(t, err, agentactivity.ErrNotFound)

	_, err = f.usecase.ReadToolPayload(f.ctx, chatID, "t1", "request")
	assert.ErrorIs(t, err, agentactivity.ErrNotFound)
}

func TestReadToolPayload_RefusesAChatThatDoesNotExist(t *testing.T) {
	f := newFixture(t)

	_, err := f.usecase.ReadToolPayload(f.ctx, "no-such-chat", "t1", "request")

	require.Error(t, err)
}
