package chat_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// ─── from observation_test.go ─────────────────────────────────────────

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

func textsOfPage(items []domain.LedgerMessage) []string {
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

// ─── from hook_delivery_test.go ───────────────────────────────────────

func TestIngestHookDelivery_DuplicatePOSTMutatesLedgerOnce(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	userDelivery := uuid.NewString()
	userPayload := mustJSON(t, map[string]any{"prompt": "exactly once"})

	for range 2 {
		require.NoError(t, f.usecase.IngestHookDelivery(
			f.ctx, "ws1", userDelivery, runnerID, "codex", "user_prompt", userPayload,
		))
	}
	stopDelivery := uuid.NewString()
	stopPayload := mustJSON(t, map[string]any{"last_assistant_message": "one reply"})
	for range 2 {
		require.NoError(t, f.usecase.IngestHookDelivery(
			f.ctx, "ws1", stopDelivery, runnerID, "codex", "turn_stop", stopPayload,
		))
	}
	f.wait()

	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 100)
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	assert.Equal(t, "exactly once", page.Items[0].Text)
	assert.Equal(t, "one reply", page.Items[1].Text)
	assert.False(t, f.chat(t, chatID).Working)
}

func TestIngestHookDelivery_RejectsUUIDReuseWithDifferentPayload(t *testing.T) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "codex")
	deliveryID := uuid.NewString()
	require.NoError(t, f.usecase.IngestHookDelivery(
		f.ctx, "ws1", deliveryID, runnerID, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "first"}),
	))

	err := f.usecase.IngestHookDelivery(
		f.ctx, "ws1", deliveryID, runnerID, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "different"}),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "different payload")
}

func TestRegression_IngestHookDelivery_ARetriedDeliveryIDRunsItsEffectsOnce(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	deliveryID := uuid.NewString()
	payload := mustJSON(t, map[string]any{"last_assistant_message": "the reply"})

	for range 3 {
		require.NoError(t, f.usecase.IngestHookDelivery(
			f.ctx, "", deliveryID, runnerID, "claude", "turn_stop", payload,
		))
	}
	f.wait()

	turns, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, turns, 1)
	assert.Equal(t, "the reply", turns[0].Text)
}

func TestIngestHookDelivery_DistinctDeliveryIDsAreDistinctTurns(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	payload := mustJSON(t, map[string]any{"last_assistant_message": "same words"})

	for range 2 {
		require.NoError(t, f.usecase.IngestHookDelivery(
			f.ctx, "", uuid.NewString(), runnerID, "claude", "turn_stop", payload,
		))
	}
	f.wait()

	turns, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	assert.Len(t, turns, 2)
}

func TestIngestHookDelivery_RefusesADeliveryIDThatIsNotACanonicalUUID(t *testing.T) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")

	for _, id := range []string{"", "not-a-uuid", "  " + uuid.NewString(), strings.ToUpper(uuid.NewString())} {
		err := f.usecase.IngestHookDelivery(
			f.ctx, "", id, runnerID, "claude", "turn_stop", mustJSON(t, map[string]any{}),
		)
		assert.Error(t, err, "delivery id %q", id)
	}
}

func TestIngestHookDelivery_AnUnknownRunnerIsDropped(t *testing.T) {
	f := newFixture(t)

	err := f.usecase.IngestHookDelivery(f.ctx, "", uuid.NewString(), uuid.NewString(),
		"claude", "turn_stop", mustJSON(t, map[string]any{"last_assistant_message": "x"}))

	assert.NoError(t, err)
}

func TestIngestHookDelivery_UsesTheRouteScopeWhenTheRunnerIsNotYetPersisted(t *testing.T) {
	f := newFixture(t)

	err := f.usecase.IngestHookDelivery(f.ctx, "ws1", uuid.NewString(), uuid.NewString(),
		"claude", "session_start", mustJSON(t, map[string]any{"session_id": "s1"}))

	assert.NoError(t, err)
}

func TestRegression_IngestHookDelivery_TheJournalIsBoundedInMemoryAndOnDisk(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	total := 10 * agentusecase.HookDeliveryPruneEvery
	guarded := total / 2
	ids := make([]string, 0, total)

	for i := range total {
		id := uuid.NewString()
		ids = append(ids, id)
		event, payload := boundedJournalDelivery(t, i, guarded, total-1)
		require.NoError(t, f.usecase.IngestHookDelivery(
			f.ctx, "", id, runnerID, "claude", event, payload,
		))
	}
	f.wait()

	assert.LessOrEqual(t, agentusecase.HookDeliveryMarkerCount(f.usecase.TurnUsecase),
		agentusecase.HookDeliveryCompletedMax, "the in-memory completion map must be capped")
	dir := filepath.Join(f.ws.chatsDir, agentusecase.HookDeliveryDirName, runnerID)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(entries), agentusecase.HookDeliveryJournalMax,
		"the on-disk runner journal must be capped")

	replayed := ids[guarded]
	require.False(t, agentusecase.HookDeliveryMarked(f.usecase.TurnUsecase, replayed),
		"the guarded delivery must have been evicted from memory, or the replay proves nothing")
	require.FileExists(t, filepath.Join(dir, replayed+".json"),
		"the guarded delivery must still be on disk, or there is nothing left to answer the replay")

	before, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, before, 2)
	require.Equal(t, "the guarded reply", before[0].Text)
	_, guardedPayload := boundedJournalDelivery(t, guarded, guarded, total-1)
	require.NoError(t, f.usecase.IngestHookDelivery(
		f.ctx, "", replayed, runnerID, "claude", "turn_stop", guardedPayload,
	))
	f.wait()

	after, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, before, after,
		"a delivery evicted from memory is still done on disk: nothing may be appended or resequenced")
}

func boundedJournalDelivery(
	t *testing.T,
	index, guarded, last int,
) (event string, payload []byte) {
	t.Helper()
	if index == guarded {
		return "turn_stop", mustJSON(t, map[string]any{"last_assistant_message": "the guarded reply"})
	}
	if index == last {
		return "turn_stop", mustJSON(t, map[string]any{"last_assistant_message": "the later reply"})
	}
	return "not_a_declared_event", mustJSON(t, map[string]any{"filler": index})
}

func TestRegression_IngestHookDelivery_AFailedCompletionDoesNotRelocateTheTurnOnReplay(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	firstID := uuid.NewString()
	firstPayload := mustJSON(t, map[string]any{"last_assistant_message": "the first reply"})
	var syncs atomic.Int64
	agentusecase.SetHookDeliveryDirSync(f.usecase.TurnUsecase, func(string) error {
		if syncs.Add(1) != 2 {
			return nil
		}
		return errors.New("injected hook delivery dir fsync failure")
	})

	require.NoError(t, f.usecase.IngestHookDelivery(
		f.ctx, "", firstID, runnerID, "claude", "turn_stop", firstPayload,
	))
	f.wait()
	require.Equal(t, int64(2), syncs.Load(), "the fault must have landed on the completion write")
	require.False(t, agentusecase.HookDeliveryMarked(f.usecase.TurnUsecase, firstID),
		"a completion whose durable write failed must not be marked done in memory")

	require.NoError(t, f.usecase.IngestHookDelivery(
		f.ctx, "", uuid.NewString(), runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "the second reply"}),
	))
	f.wait()
	before, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, before, 2)
	require.Equal(t, "the first reply", before[0].Text)

	require.NoError(t, f.usecase.IngestHookDelivery(
		f.ctx, "", firstID, runnerID, "claude", "turn_stop", firstPayload,
	),
		"a delivery whose effects already committed is done, however its marker fared")
	f.wait()

	after, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, before, after,
		"replaying the turn would bump its Seq and relocate it to the end of the log")
	replayErr := f.usecase.IngestHookDelivery(
		f.ctx, "", firstID, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "a different reply"}),
	)
	require.Error(t, replayErr)
	assert.Contains(t, replayErr.Error(), "different payload")
}

func TestRegression_IngestHookDelivery_AnIdleRunnerDirectoryIsReaped(t *testing.T) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")
	root := filepath.Join(f.ws.chatsDir, agentusecase.HookDeliveryDirName)
	stale := filepath.Join(root, uuid.NewString())
	require.NoError(t, os.MkdirAll(stale, 0o700))
	idle := time.Now().Add(-2 * agentusecase.HookDeliveryJournalMaxAge)
	require.NoError(t, os.Chtimes(stale, idle, idle))

	for i := range agentusecase.HookDeliveryPruneEvery {
		require.NoError(t, f.usecase.IngestHookDelivery(
			f.ctx, "", uuid.NewString(), runnerID, "claude", "not_a_declared_event",
			mustJSON(t, map[string]any{"filler": i}),
		))
	}
	f.wait()

	assert.NoDirExists(t, stale, "a runner directory silent past the max age must be reaped whole")
	assert.DirExists(t, filepath.Join(root, runnerID), "the live runner's directory must survive")
}

func TestRegression_IngestHookDelivery_PruningNeverRemovesAnInFlightRecord(t *testing.T) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")
	dir := filepath.Join(f.ws.chatsDir, agentusecase.HookDeliveryDirName, runnerID)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	inFlight := make([]string, 0, agentusecase.HookDeliveryJournalMax)
	stale := time.Now().Add(-time.Hour)

	for range agentusecase.HookDeliveryJournalMax - agentusecase.HookDeliveryPruneEvery/2 {
		id := uuid.NewString()
		inFlight = append(inFlight, id)
		require.NoError(t, agentusecase.PlantPendingHookDelivery(dir, id, stale))
	}
	for i := range agentusecase.HookDeliveryPruneEvery {
		require.NoError(t, f.usecase.IngestHookDelivery(
			f.ctx, "", uuid.NewString(), runnerID, "claude", "not_a_declared_event",
			mustJSON(t, map[string]any{"filler": i}),
		))
	}
	f.wait()

	for _, id := range inFlight {
		require.FileExists(t, filepath.Join(dir, id+".json"),
			"an in-flight delivery is the one thing the prune may never take")
	}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(entries), agentusecase.HookDeliveryJournalMax)
}

// ─── from turn_stall_test.go ──────────────────────────────────────────

const codexUsageLimitScreen = "" +
	"■ You've hit your usage limit. Upgrade to Pro (https://chatgpt.com/explore/pro), visit\n" +
	"https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Aug 22nd, 2026\n" +
	"12:30 PM.\n" +
	"\n" +
	"› Implement {feature}\n" +
	"  ⏎ send   ⌃J newline   ⌃T transcript   ⌃C quit"

const codexUsageLimitSentence = "" +
	"■ You've hit your usage limit. Upgrade to Pro (https://chatgpt.com/explore/pro), visit " +
	"https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Aug 22nd, 2026 " +
	"12:30 PM."

func TestRegression_StalledTurnIsClosedAndTheChatSaysWhy(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")
	prompt(t, f, runnerID, "codex", "please do the thing")
	require.True(t, f.chat(t, chatID).Working, "the user's prompt must have opened a turn")

	notice, ok := f.usecase.MatchTerminalNotice(f.ctx, "codex", codexUsageLimitScreen)
	require.True(t, ok, "the shipped codex descriptor must recognise its own usage-limit banner")

	agentusecase.CloseStalledTurn(f.usecase.TurnUsecase, f.ctx, agentusecase.Stall{
		ChatID:      chatID,
		WorkspaceID: "ws1",
		ProviderID:  "codex",
		RunnerID:    runnerID,
		SessionID:   "sess-1",
		Notice:      notice,
	})
	f.wait()

	assert.False(t, f.chat(t, chatID).Working, "the wedged spinner must stop")

	rows, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	var notices []domain.ActivityTurn
	for _, r := range rows {
		if r.Role == domain.TurnRoleNotice {
			notices = append(notices, r)
		}
	}
	require.Len(t, notices, 1, "exactly one notice turn, carrying the provider's words")
	assert.Equal(t, codexUsageLimitSentence, notices[0].Text)
	assert.Contains(t, notices[0].Text, "Aug 22nd, 2026 12:30 PM",
		"the reset time is the half of the sentence the user actually needs")
	assert.Equal(t, "codex", notices[0].ProviderID)
	assert.Equal(t, runnerID, notices[0].RunnerID)
	assert.Equal(t, "sess-1", notices[0].SessionID)
}

func TestUsecase_CloseStalledTurn_WritesNoNoticeWhenThereWasNoTurnToClose(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")
	require.False(t, f.chat(t, chatID).Working, "no prompt was sent: there is no open turn")

	notice, ok := f.usecase.MatchTerminalNotice(f.ctx, "codex", codexUsageLimitScreen)
	require.True(t, ok)
	agentusecase.CloseStalledTurn(f.usecase.TurnUsecase, f.ctx, agentusecase.Stall{
		ChatID: chatID, WorkspaceID: "ws1", ProviderID: "codex",
		RunnerID: runnerID, SessionID: "sess-1", Notice: notice,
	})
	f.wait()

	rows, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	for _, r := range rows {
		assert.NotEqual(t, domain.TurnRoleNotice, r.Role, "nothing was closed, so nothing is explained")
	}
}

func TestUsecase_MatchTerminalNotice_ResolvesTheShippedDescriptor(t *testing.T) {
	f := newFixture(t)

	notice, ok := f.usecase.MatchTerminalNotice(f.ctx, "codex", codexUsageLimitScreen)

	require.True(t, ok)
	assert.Equal(t, engineagents.TerminalNoticeUsageLimit, notice.Kind)
	assert.True(t, notice.EndsTurn, "the banner is painted because the attempt ended")
	assert.Equal(t, codexUsageLimitSentence, notice.Text)

	assert.NotContains(t, notice.Text, "Implement {feature}")
	assert.NotContains(t, notice.Text, "transcript")
}

func TestUsecase_MatchTerminalNotice_ClaudeDeclaresNone(t *testing.T) {
	f := newFixture(t)

	_, ok := f.usecase.MatchTerminalNotice(f.ctx, "claude", codexUsageLimitScreen)

	assert.False(t, ok)
}

func TestUsecase_MatchTerminalNotice_UnknownProviderIsSilent(t *testing.T) {
	f := newFixture(t)

	_, ok := f.usecase.MatchTerminalNotice(f.ctx, "telepathy", codexUsageLimitScreen)

	assert.False(t, ok)
}

func TestUsecase_MatchTerminalNotice_OrdinaryScreenIsNotANotice(t *testing.T) {
	f := newFixture(t)

	_, ok := f.usecase.MatchTerminalNotice(f.ctx, "codex",
		"› Explain this codebase\n  ⏎ send   ⌃J newline")

	assert.False(t, ok)
}

func TestUsecase_OpenWork_ReportsAToolCallTheProviderNeverClosed(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")
	prompt(t, f, runnerID, "codex", "please do the thing")

	open, err := f.usecase.OpenWork(f.ctx, chatID)
	require.NoError(t, err)
	require.False(t, open, "nothing has been started yet")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "tool_pre",
		mustJSON(t, map[string]any{
			"session_id": "sess-1", "tool_use_id": "tool-1", "tool_name": "Bash",
			"tool_input": map[string]any{"command": "sleep 600"},
		})))
	f.wait()

	open, err = f.usecase.OpenWork(f.ctx, chatID)
	require.NoError(t, err)
	assert.True(t, open, "tool_pre arrived and tool_post has not: the CLI is working")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "tool_post",
		mustJSON(t, map[string]any{
			"session_id": "sess-1", "tool_use_id": "tool-1", "tool_name": "Bash",
			"tool_input": map[string]any{"command": "sleep 600"}, "tool_response": "done",
		})))
	f.wait()

	open, err = f.usecase.OpenWork(f.ctx, chatID)
	require.NoError(t, err)
	assert.False(t, open)
}

func TestUsecase_OpenWork_ReportsASubagentTheProviderNeverStopped(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")
	prompt(t, f, runnerID, "codex", "please do the thing")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "subagent_pre",
		mustJSON(t, map[string]any{
			"session_id": "sess-1", "agent_id": "sub-1", "agent_type": "explorer",
		})))
	f.wait()

	open, err := f.usecase.OpenWork(f.ctx, chatID)
	require.NoError(t, err)
	assert.True(t, open)

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "subagent_post",
		mustJSON(t, map[string]any{
			"session_id": "sess-1", "agent_id": "sub-1", "agent_type": "explorer",
		})))
	f.wait()

	open, err = f.usecase.OpenWork(f.ctx, chatID)
	require.NoError(t, err)
	assert.False(t, open)
}

func TestUsecase_MatchTerminalNotice_SurvivesANarrowPane(t *testing.T) {
	f := newFixture(t)
	rows := wrapAt(codexUsageLimitSentence, 24)
	require.Len(t, rows, 8, "the fixture must sit exactly on the capture bound")
	for _, row := range rows {
		require.NotContains(t, row, "You've hit your usage limit",
			"the fixture must genuinely split the needle across rows")
	}

	notice, ok := f.usecase.MatchTerminalNotice(f.ctx, "codex", strings.Join(rows, "\n"))

	require.True(t, ok)
	assert.Equal(t, codexUsageLimitSentence, notice.Text)
}

func wrapAt(text string, width int) []string {
	var rows []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) <= width:
			line += " " + word
		default:
			rows = append(rows, line)
			line = word
		}
	}
	if line != "" {
		rows = append(rows, line)
	}
	return rows
}

type orderLog struct {
	mu   sync.Mutex
	seen []string
}

func (l *orderLog) note(what string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seen = append(l.seen, what)
}

func (l *orderLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.seen...)
}

type orderedChats struct {
	agentchat.EventStore
	log *orderLog
}

func (o orderedChats) AbandonTurn(
	ctx context.Context, chatID string, now time.Time,
) (domain.Chat, error) {
	o.log.note("abandon-turn")
	return o.EventStore.AbandonTurn(ctx, chatID, now)
}

type orderedActivity struct {
	agentactivity.EventStore
	log *orderLog
}

func (o orderedActivity) AppendTurn(ctx context.Context, in agentactivity.TurnInput) error {
	if in.Role == domain.TurnRoleNotice {
		o.log.note("notice-appended")
	}
	return o.EventStore.AppendTurn(ctx, in)
}

func TestRegression_StallNoticeIsDurableBeforeTheIdleEdgeIsPublished(t *testing.T) {
	log := &orderLog{}
	f, _, _ := newFixtureUsing(t,
		func(real agentchat.EventStore) agentchat.EventStore {
			return orderedChats{EventStore: real, log: log}
		},
		nil,
		"",
		func(real agentactivity.EventStore) agentactivity.EventStore {
			return orderedActivity{EventStore: real, log: log}
		},
	)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")
	prompt(t, f, runnerID, "codex", "please do the thing")
	require.True(t, f.chat(t, chatID).Working)

	notice, ok := f.usecase.MatchTerminalNotice(f.ctx, "codex", codexUsageLimitScreen)
	require.True(t, ok)
	agentusecase.CloseStalledTurn(f.usecase.TurnUsecase, f.ctx, agentusecase.Stall{
		ChatID: chatID, WorkspaceID: "ws1", ProviderID: "codex",
		RunnerID: runnerID, SessionID: "sess-1", Notice: notice,
	})
	f.wait()

	assert.Equal(t, []string{"notice-appended", "abandon-turn"}, log.all(),
		"the explanation must be durable before anything publishes the idle edge")
}

// ─── from choice_test.go ──────────────────────────────────────────────

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

// ─── from injected_prompt_test.go ─────────────────────────────────────

const taskNotificationPrompt = `<task-notification>
<task-id>aa3b60603214670cc</task-id>
<tool-use-id>toolu_01CZ…</tool-use-id>
<output-file>…</output-file>
<status>completed</status>
<summary>Agent "Reply with PONG" finished</summary>
<note>A task-notification fires each time this agent stops with no live background children of its own. …</note>
<result>PONG</result>
<usage><subagent_tokens>18471</subagent_tokens><tool_uses>0</tool_uses><duration_ms>1337</duration_ms></usage>
</task-notification>`

const (
	crowbarDeliveredPrompt = "Launch exactly one general-purpose subagent with the Agent tool. …"
	composerTypedPrompt    = "say only the word ACK"
)

func TestRegression_HarnessInjectedPromptIsRecordedAsHarnessNotUser(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": taskNotificationPrompt})))
	f.wait()

	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, page.Items, 1, "the injected prompt is recorded, never dropped")
	assert.Equal(t, domain.TurnRoleHarness, page.Items[0].Role)
	assert.Equal(t, taskNotificationPrompt, page.Items[0].Text,
		"recorded verbatim: it is the context the next reply answers")
}

func TestRegression_HarnessInjectedPromptStillOpensTheTurn(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.False(t, f.chat(t, chatID).Working, "a fresh chat is not Working")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": taskNotificationPrompt})))

	working := f.chat(t, chatID)
	assert.True(t, working.Working, "an injected prompt still opens the turn")
	require.NotNil(t, working.CurrentTurnStarted)
}

func TestRegression_RealUserPromptsWithNoSourceKeyStayTheUsers(t *testing.T) {
	for name, prompt := range map[string]string{
		"crowbar positional delivery": crowbarDeliveredPrompt,
		"typed into the composer":     composerTypedPrompt,
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)

			chatID, runnerID := f.spawn(t, "claude")

			require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
				mustJSON(t, map[string]any{"prompt": prompt})))
			f.wait()

			page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 0)
			require.NoError(t, err)
			require.Len(t, page.Items, 1)
			assert.Equal(t, domain.TurnRoleUser, page.Items[0].Role)
			assert.Equal(t, prompt, page.Items[0].Text)
		})
	}
}

func TestIngestHook_UserPrompt_ProviderDeclaringNoInjectedPromptsRecordsEverythingAsUser(
	t *testing.T,
) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": taskNotificationPrompt})))
	f.wait()

	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, domain.TurnRoleUser, page.Items[0].Role)
}

func TestIngestHook_UserPrompt_HarnessInjectionNeverBecomesTheChatTitle(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.bc.reset()

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": taskNotificationPrompt})))

	assert.NotContains(t, f.chat(t, chatID).Title, "task-notification")

	assert.Equal(t, []string{"turn_started"}, f.bcKinds(t))

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": composerTypedPrompt})))
	assert.Equal(t, "say only the word ACK", f.chat(t, chatID).Title)
}

func TestRegression_ChatLogDoesNotServeAHarnessTurnAsTheUsers(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": taskNotificationPrompt})))
	f.wait()

	turns, err := f.usecase.ReadChatLog(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, turns, 1)
	assert.NotEqual(t, "user", turns[0].Speaker)
	assert.Contains(t, turns[0].Speaker, "harness")
	assert.Contains(t, turns[0].Speaker, "NOT the user")

	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)
	assert.Contains(t, handoff, "claude harness (injected, NOT the user):")
}

// ─── from async_work_test.go ──────────────────────────────────────────

// TestSwitchProvider_BackgroundWork_WaitsForAuthoritativeIdle proves a provider
// switch cannot treat turn_stop as "the CLI is done" when that same hook reports
// work still running. There is no sleep or projection polling: the switch announces
// that it is parked, and only the later authoritative zero-level hook releases it.
func TestSwitchProvider_BackgroundWork_WaitsForAuthoritativeIdle(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	oldTerm := f.runner(t, runnerID).TerminalSession
	f.announce(t, runnerID, "s1")
	prompt(t, f, runnerID, "claude", "launch a background subagent")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "Launched.", 1)))
	f.wait()
	require.True(t, f.chat(t, chatID).Working, "precondition: async work keeps the chat live")

	killed := terminateSignal(f)
	parked := parkedOnTurn(t)
	done := make(chan switchResult, 1)
	go func() {
		id, err := f.usecase.SwitchProvider(context.Background(), chatID, "codex")
		done <- switchResult{runnerID: id, err: err}
	}()

	select {
	case <-parked:
	case sess := <-killed:
		t.Fatalf("the outgoing CLI (%s) was terminated while its background work was live", sess)
	case got := <-done:
		t.Fatalf("the switch returned while background work was live: %+v", got)
	}
	require.Empty(t, f.term.terminatedIDs(), "background work must keep the outgoing TUI alive")

	// Claude's later status hook restates the level at zero. Only this semantic
	// transition — not elapsed time and not projection convergence — may release it.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "The background subagent finished.", 0)))

	got := <-done
	require.NoError(t, got.err)
	f.wait()
	require.Contains(t, f.term.terminatedIDs(), oldTerm)
}

// stopPayload is a real claude 2.1.212 Stop hook payload carrying `running` background
// tasks — the shape traced live while a background subagent was working. tasks is how
// many entries claude reports STILL OUTSTANDING as it goes quiet.
func stopPayload(t *testing.T, message string, tasks int) []byte {
	t.Helper()
	bg := make([]any, 0, tasks)
	for range tasks {
		bg = append(bg, map[string]any{
			"id":         "abbe4333c2384e2dc",
			"type":       "subagent",
			"status":     "running",
			"agent_type": "general-purpose",
		})
	}
	return mustJSON(t, map[string]any{
		"session_id":             "s1",
		"last_assistant_message": message,
		"background_tasks":       bg,
		"session_crons":          []any{},
	})
}

// TestRegression_TurnStopWithBackgroundSubagent_KeepsChatWorking is THE BUG, end to end
// through the real usecase, the real descriptor and the real aggregate — the hook payload
// in, the spinner out.
//
// Traced against claude 2.1.212: the CLI spawns a BACKGROUND subagent, then goes quiet
// waiting to be re-invoked when it reports back — which ends its turn for real, and it
// fires Stop right there, ~18 seconds before the subagent actually finished. Crowbar read
// that Stop as "done" and darkened the spinner on a chat whose agent was still working.
// The user thinks it died.
//
// This is the guard on the wiring between them: the level claude reports on its Stop must
// travel from the hook payload into the fold. Dropping it on the floor in the usecase —
// passing 0 instead of ev.AsyncWork — reproduces the original bug exactly, with every
// other test still green.
func TestRegression_TurnStopWithBackgroundSubagent_KeepsChatWorking(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "launch a background subagent and wait for it"})))
	require.True(t, f.chat(t, chatID).Working, "precondition: the turn is open")

	// claude hands the work off and ends its turn — with one subagent still running.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "Launched. The subagent is running in the background.", 1)))
	f.wait()

	chat := f.chat(t, chatID)
	require.True(t, chat.Working,
		"the spinner must KEEP SPINNING: the turn ended but a background subagent is still working")
	require.Nil(t, chat.CurrentTurnStarted, "the turn itself really did end")
	require.Equal(t, 1, chat.AsyncWork)
}

// TestRegression_BackgroundSubagentFinishes_StopsChatWorking is the other half, and the
// one that keeps the fix from becoming a WORSE bug than the one it fixes: the spinner has
// to actually STOP. A permanently-spinning spinner lies forever, and this is an
// event-sourced aggregate, so it would survive restarts.
//
// Traced: when the subagent reports back claude re-invokes itself (a UserPromptSubmit
// carrying a <task-notification>), answers, and ends THAT turn with background_tasks: [].
func TestRegression_BackgroundSubagentFinishes_StopsChatWorking(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "launch a background subagent"})))
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "Launched.", 1)))
	f.wait()
	require.True(t, f.chat(t, chatID).Working, "precondition: spinning on the subagent")

	// The subagent reports back: claude re-invokes itself and ends the turn with nothing left.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "<task-notification><task-id>abbe4333c2384e2dc</task-id>"})))
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "The subagent finished.", 0)))
	f.wait()

	chat := f.chat(t, chatID)
	require.False(t, chat.Working, "once the work is done the spinner MUST stop — no stuck-on")
	require.Equal(t, 0, chat.AsyncWork)
}

// TestRegression_ConcurrentBackgroundSubagentsDrain_StopsChatWorking is the traced
// multi-subagent case: three at once, and claude RESTATES the whole list on every Stop as
// it drains ([running x3] → [running x4] → [running] → []). The level follows it down and
// lands idle — no pairing, no arithmetic, nothing to leak.
func TestRegression_ConcurrentBackgroundSubagentsDrain_StopsChatWorking(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")

	// Each restatement claude actually emitted, in order. A subagent spawning MORE
	// subagents (3 → 4) is why this must never be a decrementing counter.
	for _, outstanding := range []int{3, 4, 1, 0} {
		require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
			mustJSON(t, map[string]any{"prompt": "turn"})))
		require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
			stopPayload(t, "restated", outstanding)))
		f.wait()

		if outstanding > 0 {
			require.Truef(t, f.chat(t, chatID).Working,
				"%d subagents still running: must keep spinning", outstanding)
		}
	}

	require.False(t, f.chat(t, chatID).Working, "drained to zero: the spinner must stop")
}

// TestRegression_InterruptedTurnThenNewPrompt_DoesNotStrandSpinner is the case the
// PREVIOUS attempt broke, and it is pre-existing in shape.
//
// Traced: an INTERRUPT (ESC) fires NO HOOK AT ALL — not Stop, not Notification, nothing —
// so the turn it interrupted is never closed by the CLI, and any async work it announced
// is never retired. The previous attempt counted work_begin/work_end edges, so an
// interrupt during background work stranded the count at 3 and spun that chat FOREVER.
//
// Here the next prompt supersedes both: a new turn zeroes the level, and that turn's own
// Stop settles it. The spinner comes back to the truth.
func TestRegression_InterruptedTurnThenNewPrompt_DoesNotStrandSpinner(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")

	// A turn that ended with background work outstanding...
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "launch three background subagents"})))
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "Launched three.", 3)))
	f.wait()
	require.True(t, f.chat(t, chatID).Working, "precondition: spinning on 3 subagents")

	// ...the user hits ESC. NOTHING arrives — that is the whole point; no hook exists.
	// Then they type again. This is the only edge that can heal it.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "hi"})))
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "hello", 0)))
	f.wait()

	chat := f.chat(t, chatID)
	require.False(t, chat.Working,
		"an interrupt must not strand the spinner: the next completed turn settles it")
	require.Equal(t, 0, chat.AsyncWork)
}

// TestRegression_KilledCLIWithBackgroundWork_DoesNotSpinForever is the stuck-on case that
// the hook surface cannot fix, and the reconcile must.
//
// Traced: SIGKILL mid-background-work sends NO SessionEnd and NO final Stop. The last word
// on the aggregate is a turn_stop reporting work still running, with nobody left alive to
// restate it — and in an event-sourced aggregate that word outlives the daemon. The boot
// reconcile (a dead PTY cannot still be working) is what clears it.
func TestRegression_KilledCLIWithBackgroundWork_DoesNotSpinForever(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "launch a background subagent"})))
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "Launched.", 1)))
	f.wait()
	require.True(t, f.chat(t, chatID).Working, "precondition: spinning on announced work")
	require.Nil(t, f.chat(t, chatID).CurrentTurnStarted,
		"and NOT because a turn is open — the turn closed; only the work keeps it lit")

	// The CLI dies with the daemon. No Stop is coming, ever.
	f.term.dieWithDaemon()
	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	chat := f.chat(t, chatID)
	require.False(t, chat.Working,
		"a dead CLI's announced work must not spin the chat forever across a restart")
	require.Equal(t, 0, chat.AsyncWork)
}

// TestCodexTurnStop_NeverReportsAsyncWork is the PROVIDER-AGNOSTIC requirement through the
// live usecase: codex maps no async_work, so even a Stop payload that happens to carry a
// background_tasks array leaves it turn-only and bit-identical to before this existed.
func TestCodexTurnStop_NeverReportsAsyncWork(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "s1")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "do a thing"})))
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "turn_stop",
		stopPayload(t, "done", 3)))
	f.wait()

	chat := f.chat(t, chatID)
	require.False(t, chat.Working, "codex maps no async_work: turn_stop is simply idle")
	require.Equal(t, 0, chat.AsyncWork, "an unmapped field must never be counted")
}
