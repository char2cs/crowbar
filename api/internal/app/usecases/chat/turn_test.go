package chat_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// ─── from observation_test.go ─────────────────────────────────────────

func hook(t *testing.T, f testFixture, runnerID, provider, kind string, payload map[string]any) {
	t.Helper()
	f.withTrackedSession(runnerID, payload)
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, provider, kind, mustJSON(t, payload)))
	f.wait()
}

// hookAPI is hook() delivered on an API-marked ctx — inflight.WithAPITransport,
// the SAME marker pumpAPIConn's own IngestHook calls carry in production (see
// ingest.go's channelFor). Use it for a payload shaped the way a provider's
// api: channel block actually maps it (codex's threadId/item.*/turn.* — see
// codex.yaml) rather than hook()'s hooks-shaped default; delivering an
// api-shaped payload through hook() instead resolves against the wrong
// channel block (or no block at all) and silently drops or misparses it.
func hookAPI(t *testing.T, f testFixture, runnerID, provider, kind string, payload map[string]any) {
	t.Helper()
	require.NoError(t, f.usecase.IngestHook(
		inflight.WithAPITransport(f.ctx), runnerID, provider, kind, mustJSON(t, payload)))
	f.wait()
}

// codexTurnStop delivers a codex turn_stop the way pumpAPIConn actually
// receives one in production: api-shaped (threadId/turn.items[type=
// agentMessage].text — codex.yaml's turn_stop:api: block), on an API-marked
// ctx. turn()'s own flat last_assistant_message is claude's hooks shape and
// silently resolves to an empty message under codex's api: block.
func codexTurnStop(t *testing.T, f testFixture, runnerID, sessionID, message string) {
	t.Helper()
	hookAPI(t, f, runnerID, "codex", "turn_stop", map[string]any{
		"threadId": sessionID,
		"turn": map[string]any{
			"items": []any{
				map[string]any{"type": "agentMessage", "text": message},
			},
		},
	})
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

// TestRegression_UserPromptHookRestoresTheDurableAttachmentRef pins the bug a
// live excalidraw-attachment send surfaced: a user_prompt hook's own message
// IS the text the CLI actually received, which is the file's real ABSOLUTE
// path in the durable store (materializeAttachmentsForDispatch's rewrite)
// whenever the prompt referenced an attachment. Recording that verbatim as
// the ledger's turn text broke two things at once — the asset-serving
// endpoint can't resolve a raw filesystem path, so the picture never
// rendered again, and that local path leaked into what the user reads as
// their own sent message. The ledger must always hold the logical
// chats/<chatID>/attachments/<file> reference.
func TestRegression_UserPromptHookRestoresTheDurableAttachmentRef(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	absPath := filepath.ToSlash(filepath.Join(
		worktreepath.AttachmentsDir(f.ws.chatsDir, chatID), "diagram.png"))
	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt,
		map[string]any{"prompt": "check this out ![diagram](" + absPath + ")"})

	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 10)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t,
		"check this out ![diagram](chats/"+chatID+"/attachments/diagram.png)",
		page.Items[0].Text)
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

// TestObservation_ANestedSubagentsToolCallsAndReplyAreRecorded drives the
// FULL nested-subagent mechanism end to end, through the real descriptor
// (codex.yaml) and the real event-sourced activity store — not a mocked
// port. codex's own collabAgentToolCall(spawnAgent) completion is what OPENS
// the subagent (item.receiverThreadIds[0], the mapping's dynamic-key
// selector's own motivating field — see mapping.go and codex.yaml's
// nested_session_id note); the spawned agent's own child thread then runs
// its own tool calls and its own turn_stop, all carrying the CHILD's session
// id, which routeNestedSubagentEvent must route into the subagent's own
// nested activity rather than drop (namesAnotherConversation's own job) or
// bleed into the chat's top-level turn (the bug
// TestRegression_AChildThreadsTurnStopNeverClosesThisChatsTurn guards).
func TestObservation_ANestedSubagentsToolCallsAndReplyAreRecorded(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "thread-main")

	// The PARENT thread's own spawnAgent tool call completes, naming the
	// child thread it just created (item.receiverThreadIds[0]) — this is
	// what opens the nested subagent for real. nested_session_id only
	// resolves off codex's api: channel block (see codex.yaml's own comment
	// on tool_post — the hooks: block maps no such field at all, since no
	// PostToolUse capture of a collab-agent completion has ever been taken),
	// so this — like every event below — must arrive on an API-marked ctx,
	// api-shaped, exactly as codex's real collabAgentToolCall traffic does.
	hookAPI(t, f, runnerID, "codex", "tool_post", map[string]any{
		"threadId": "thread-main",
		"item": map[string]any{
			"type": "collabAgentToolCall", "id": "spawn-1", "tool": "spawnAgent",
			"receiverThreadIds": []any{"thread-child"},
		},
	})

	subs, err := f.activity.Subagents(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	assert.Equal(t, "thread-child", subs[0].ID)
	assert.Nil(t, subs[0].EndedAt, "opened, not yet closed")

	// The CHILD thread's own tool call — threadId names the CHILD, not the
	// parent — must be routed into the subagent's nested activity.
	hookAPI(t, f, runnerID, "codex", "tool_pre", map[string]any{
		"threadId": "thread-child",
		"item":     map[string]any{"type": "commandExecution", "id": "child-tool-1", "command": "echo hi"},
	})
	hookAPI(t, f, runnerID, "codex", "tool_post", map[string]any{
		"threadId": "thread-child",
		"item": map[string]any{
			"type": "commandExecution", "id": "child-tool-1", "command": "echo hi",
			"aggregatedOutput": "hi",
		},
	})

	// The CHILD thread's own turn closing is what CLOSES the subagent and
	// records its own final reply.
	hookAPI(t, f, runnerID, "codex", "turn_stop", map[string]any{
		"threadId": "thread-child",
		"turn": map[string]any{
			"items": []any{map[string]any{"type": "agentMessage", "text": "done"}},
		},
	})

	calls, err := f.activity.ToolCalls(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	// Two rows: the PARENT's own spawnAgent call (already-working, ordinary
	// top-level visibility, unaffected by any of this — SubagentID empty)
	// and the CHILD's own nested Bash call (SubagentID set).
	require.Len(t, calls, 2)
	var parent, nested *domain.ActivityToolCall
	for i := range calls {
		switch calls[i].SubagentID {
		case "":
			parent = &calls[i]
		default:
			nested = &calls[i]
		}
	}
	require.NotNil(t, parent, "the parent's own spawnAgent tool call must still show, unaffected")
	assert.Equal(t, "spawnAgent", parent.Name)
	require.NotNil(t, nested, "the child's own tool call must be recorded")
	assert.Equal(t, "thread-child", nested.SubagentID)
	// "commandExecution", not a specific binary name: a real commandExecution
	// item carries no item.tool field at all (confirmed live — see the
	// item_started/item_completed.commandExecution.json fixtures), so
	// tool_name's first_present chain falls through to item.type, same as
	// any other unmapped codex tool call (codex.yaml's own tool_target
	// comment: "unmapped every codex tool call read as the bare word
	// 'commandExecution'").
	assert.Equal(t, "commandExecution", nested.Name)
	assert.Equal(t, domain.ToolStatusOK, nested.Status)
	assert.Empty(t, nested.TurnID, "a nested tool call has no top-level turn")

	subs, err = f.activity.Subagents(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	require.NotNil(t, subs[0].EndedAt, "the child's own turn_stop must close it")
	require.Len(t, subs[0].Messages, 1)
	assert.Equal(t, "done", subs[0].Messages[0].Text)

	// The chat's own top-level turn activity must stay untouched — the whole
	// point of ROUTING instead of bleeding through.
	turns, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, turns, "no top-level turn was ever opened by any of this")
}

// TestRegression_SpawnAgentCompletionKeepsChatWorkingForTheNestedSubagent is
// THE BUG live-reported against SubagentShelf: a codex chat ran a real
// collab_agents subagent, and the live shelf never showed it running at all.
//
// handleObservation's HookToolPost case used to call restateAsyncWork BEFORE
// openNestedSubagent — so the very read meant to notice "something is now
// open" ran one statement before the write that would have given it
// something to find. With the PARENT's own turn already closed (codex ends
// its visible turn the instant it delegates — see observation.go's own
// comment on this) and nothing else open, that ordering restated
// Working=false right as the nested subagent started, and nothing ever
// reopened it: SubagentShelf's live poll had already stopped.
func TestRegression_SpawnAgentCompletionKeepsChatWorkingForTheNestedSubagent(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "thread-main")

	hook(t, f, runnerID, "codex", "user_prompt", map[string]any{"prompt": "spawn a subagent"})
	require.True(t, f.chat(t, chatID).Working, "precondition: the turn is open")

	// codex ends its OWN visible turn the instant it delegates — nothing else
	// is open once this lands.
	hook(t, f, runnerID, "codex", "turn_stop", map[string]any{
		"session_id": "thread-main", "last_assistant_message": "Delegating now.",
	})
	require.False(t, f.chat(t, chatID).Working,
		"precondition: the parent's own turn genuinely closed, nothing else open yet")

	// The parent's own spawnAgent tool call completes, naming the child
	// thread it just created — this is what opens the nested subagent, and
	// it is the ONLY thing open at this instant. nested_session_id only
	// resolves off codex's api: channel block (see the sibling test above),
	// so this must be api-shaped on an API-marked ctx.
	hookAPI(t, f, runnerID, "codex", "tool_post", map[string]any{
		"threadId": "thread-main",
		"item": map[string]any{
			"type": "collabAgentToolCall", "id": "spawn-1", "tool": "spawnAgent",
			"receiverThreadIds": []any{"thread-child"},
		},
	})

	chat := f.chat(t, chatID)
	require.True(t, chat.Working,
		"the spinner must turn back on: a nested subagent just opened and is still running")

	subs, err := f.activity.Subagents(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	assert.Nil(t, subs[0].EndedAt, "opened, not yet closed")
}

// TestRegression_ASubagentsPromptAndReplyNeverEnterTheMainTranscript is the
// bug reported live 2026-09-23: a codex chat running with
// features.collab_agents filed its SUBAGENTS' prompts as the user's own
// messages and their answers as the main agent's, interleaved into the
// transcript (dev daemon, chat b1fb213e — three `turn_appended` role: user
// rows carrying prompts Crowbar's own agent wrote, plus four assistant
// messages under four different item ids).
//
// The precondition is the whole bug: a RESUMED chat can have no bound
// conversation at all. codex's api transport resumes through thread/resume,
// which fires no thread/started notification, so HandleSessionStart never
// runs and the runner row keeps an empty currentSessionId for its entire
// life — confirmed in the live daemon's own runner event log, which holds
// exactly one runner.started for that chat's runner and no
// runner.session_bound. namesAnotherConversation then had NOTHING to compare
// each event's session id against and waved every foreign thread through,
// including the child threads codex pushes down this same connection.
func TestRegression_ASubagentsPromptAndReplyNeverEnterTheMainTranscript(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "thread-main")
	turn(t, f, runnerID, "codex", "answered before the TUI stopped")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.term.exit(t, "term-1")
	f.wait()
	resumedID, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	resumed := f.runner(t, resumedID)
	require.Equal(t, "thread-main", resumed.LaunchSessionID)
	require.Empty(t, resumed.CurrentSession,
		"precondition: an api-transport resume announces nothing, so the runner never binds a session")

	// The parent's own spawnAgent completes, naming the child thread it just
	// created — this is what opens the nested subagent.
	hookAPI(t, f, resumedID, "codex", "tool_post", map[string]any{
		"threadId": "thread-main",
		"item": map[string]any{
			"type": "collabAgentToolCall", "id": "spawn-1", "tool": "spawnAgent",
			"receiverThreadIds": []any{"thread-child"},
		},
	})

	// Everything below names the CHILD thread: the prompt Crowbar's own agent
	// handed its subagent, the subagent's streamed answer, and the subagent's
	// own turn closing. None of it is this chat's transcript.
	hookAPI(t, f, resumedID, "codex", "user_prompt", map[string]any{
		"threadId": "thread-child",
		"item": map[string]any{
			"type": "userMessage",
			"content": []any{map[string]any{
				"type": "text", "text": "Perform a read-only test-inventory check.",
			}},
		},
	})
	hookAPI(t, f, resumedID, "codex", "message_delta", map[string]any{
		"threadId": "thread-child", "turnId": "child-turn-1",
		"itemId": "msg_child", "delta": "Top-level entries: .git, README.md",
	})
	hookAPI(t, f, resumedID, "codex", "turn_stop", map[string]any{
		"threadId": "thread-child",
		"turn": map[string]any{
			"items": []any{map[string]any{
				"type": "agentMessage", "text": "Top-level entries: .git, README.md",
			}},
		},
	})

	turns, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	for _, recorded := range turns {
		assert.NotContains(t, recorded.Text, "read-only test-inventory",
			"a prompt Crowbar's own agent wrote was filed as the user's own message")
		assert.NotContains(t, recorded.Text, "Top-level entries",
			"a subagent's answer was filed as the main agent's own")
	}

	// ...and none of that visibility is lost: the child's work stays on the
	// subagent it belongs to.
	subs, err := f.activity.Subagents(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	require.NotNil(t, subs[0].EndedAt, "the child's own turn_stop closes it")
	require.Len(t, subs[0].Messages, 1)
	assert.Equal(t, "Top-level entries: .git, README.md", subs[0].Messages[0].Text)
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

// TestObservation_CompactionPushesTheLiveEdgeDirectly pins the fix for the live
// "Compacting…" indicator: the ledger's own interruption record for a
// compaction is born already resolved (TestObservation_ACompactionOpensAndResolvesTheSameRecord,
// above — no turn is ever open for a bare-prompt /compact, so
// commands.Interrupt's idle-chat handling resolves it in the SAME event that
// creates it), so nothing reading that record can ever observe a live
// "in progress" window. This is what makes the direct compactionStatus push
// necessary at all: it is the ONLY place "compaction just started" and
// "compaction just ended" are each observable as their own moment.
func TestObservation_CompactionPushesTheLiveEdgeDirectly(t *testing.T) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")

	var mu sync.Mutex
	var calls []bool
	f.usecase.StartTerminalWaitSweep(f.ctx, nil, nil, nil,
		func(_, _ string, active bool) {
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, active)
		}, nil)

	hook(t, f, runnerID, "claude", engineagents.HookCompactPre, map[string]any{"trigger": "auto"})
	hook(t, f, runnerID, "claude", engineagents.HookCompactPost, map[string]any{"trigger": "auto"})

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []bool{true, false}, calls,
		"compact_pre must push active=true and compact_post must push active=false, in order")
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

// TestObservation_AToolPreWithNoIDIsRejectedNotRecorded replaces the former
// "anonymous tool calls do not collide" expectation: claude's real
// PreToolUse/PostToolUse payloads ALWAYS carry tool_use_id (PreToolUse.json/
// PostToolUse.json's own fixtures prove it, which is why claude.yaml's
// tool_pre/tool_post keep tool_id in required: — design spec 2.3), so a
// PreToolUse hook missing it is a malformed delivery, not a legitimate
// anonymous call. required: now rejects it outright instead of toolID()
// silently minting a fresh fallback id per call — which was worse, not
// better: two genuinely anonymous deliveries of the SAME real call would
// mint two DIFFERENT ids and its own pre/post pair would never match
// (codex.yaml's own comment on tool_id being "load-bearing").
func TestObservation_AToolPreWithNoIDIsRejectedNotRecorded(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookToolPre, map[string]any{"tool_name": "Bash"})

	calls, err := f.activity.ToolCalls(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, calls, "a tool_pre missing its required tool_id must be rejected, not recorded")
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
			f.ctx, userDelivery, runnerID, "codex", "user_prompt", userPayload,
		))
	}
	stopDelivery := uuid.NewString()
	stopPayload := mustJSON(t, map[string]any{"last_assistant_message": "one reply"})
	for range 2 {
		require.NoError(t, f.usecase.IngestHookDelivery(
			f.ctx, stopDelivery, runnerID, "codex", "turn_stop", stopPayload,
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
		f.ctx, deliveryID, runnerID, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "first"}),
	))

	err := f.usecase.IngestHookDelivery(
		f.ctx, deliveryID, runnerID, "codex", "user_prompt",
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
			f.ctx, deliveryID, runnerID, "claude", "turn_stop", payload,
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
			f.ctx, uuid.NewString(), runnerID, "claude", "turn_stop", payload,
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
			f.ctx, id, runnerID, "claude", "turn_stop", mustJSON(t, map[string]any{}),
		)
		assert.Error(t, err, "delivery id %q", id)
	}
}

func TestIngestHookDelivery_AnUnknownRunnerIsDropped(t *testing.T) {
	f := newFixture(t)

	err := f.usecase.IngestHookDelivery(f.ctx, uuid.NewString(), uuid.NewString(),
		"claude", "turn_stop", mustJSON(t, map[string]any{"last_assistant_message": "x"}))

	assert.NoError(t, err)
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

// TestRegression_CloseStalledTurnSalvagesTheAlreadyStreamedText is the same gap
// TestRegression_StopChatSalvagesTheAlreadyStreamedText already proved on StopChat's
// own door, hit through the stall sweep instead: codex streams part of a reply,
// then hits its usage limit (codex.yaml's only terminal_notices entry) before its
// own turn/completed ever arrives. CloseStalledTurn clears Working — the wedged
// spinner does stop — but unlike AbandonMessage/AbandonMessageForRunner it never
// looked at the streamed buffer, so the reply already broadcast live over
// message_delta was simply dropped. The frontend's own live bubble, matched
// against the ledger by "msg-"+item id, then never finds its row and never
// stops rendering as still in progress.
func TestRegression_CloseStalledTurnSalvagesTheAlreadyStreamedText(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")
	prompt(t, f, runnerID, "codex", "please do the thing")
	require.True(t, f.chat(t, chatID).Working, "the user's prompt must have opened a turn")

	// Codex streaming its own reply — no `final`/`index`, exactly as its own
	// descriptor maps message_delta — when the usage-limit banner appears.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "message_delta",
		mustJSON(t, map[string]any{
			"threadId": "sess-1", "turnId": "turn-1", "itemId": "reply-msg",
			"delta": "Here is the first part of the answer...",
		})))
	f.wait()

	notice, ok := f.usecase.MatchTerminalNotice(f.ctx, "codex", codexUsageLimitScreen)
	require.True(t, ok)
	agentusecase.CloseStalledTurn(f.usecase.TurnUsecase, f.ctx, agentusecase.Stall{
		ChatID: chatID, WorkspaceID: "ws1", ProviderID: "codex",
		RunnerID: runnerID, SessionID: "sess-1", Notice: notice,
	})
	f.wait()

	assert.False(t, f.chat(t, chatID).Working, "the wedged spinner must stop")

	turns, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	var salvaged *domain.ActivityTurn
	for i := range turns {
		if turns[i].Text == "Here is the first part of the answer..." {
			salvaged = &turns[i]
		}
	}
	require.NotNil(t, salvaged,
		"THE BUG: text Crowbar already streamed to the client must survive a stall, not disappear")
	assert.Equal(t, "codex", salvaged.ProviderID)
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

// This test has migrated twice as codex's descriptor grew, and each move is the
// point: it exists to prove the "unmapped kind is DROPPED, never failed"
// invariant, so it must always name a kind codex genuinely does not declare.
//
// It ran "elicitation" until a9ebb6f1 ("merge codex into one mixed-transport
// descriptor") gave codex a real elicitation: mapping — see
// TestObservation_ACodexElicitationIsRecordedAsAnInterruptionAndAPrompt. It then
// ran "tool_fail" until codex gained one of those too (item/completed gated on
// item.status: failed || declined) — see the positive case directly below.
//
// notification is what is left: codex's app-server exposes no notification of
// that shape at all, which codex.yaml records in place.
func TestObservation_ACodexChatObservesNoNotification(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	hook(t, f, runnerID, "codex", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})

	err := f.usecase.IngestHook(f.ctx, runnerID, "codex", engineagents.HookNotification,
		mustJSON(t, map[string]any{"session_id": "s1", "message": "needs your attention"}))
	f.wait()

	require.NoError(t, err, "an unmapped kind is dropped, never failed")
	assert.Empty(t, pendingChoices(t, f, chatID))
	interruptions, listErr := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, listErr)
	assert.Empty(t, interruptions)
}

// The positive case that replaced tool_fail above. A failed or declined codex
// tool used to be recorded as a silent OK: tool_post fired on item/completed
// whatever item.status said, and codex declared no tool_fail at all, so its
// error text was discarded and the row looked exactly like a success.
func TestRegression_ACodexFailedToolIsRecordedAsAnError(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	hook(t, f, runnerID, "codex", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})

	// tool_fail is api-only — codex declares no hooks: block for it at all
	// (PostToolUse always names canonical tool_post, never tool_fail — see
	// codex.yaml's own comment) — so both of these must arrive api-shaped on
	// an API-marked ctx, or tool_fail resolves to nothing and drops.
	hookAPI(t, f, runnerID, "codex", engineagents.HookToolPre, map[string]any{
		"threadId": "t1", "turnId": "tn1",
		"item": map[string]any{
			"type": "commandExecution", "id": "c1", "command": "rg --files", "status": "inProgress",
		},
	})
	hookAPI(t, f, runnerID, "codex", engineagents.HookToolFail, map[string]any{
		"threadId": "t1", "turnId": "tn1",
		"item": map[string]any{
			"type": "commandExecution", "id": "c1", "command": "rg --files",
			"status": "failed", "aggregatedOutput": "rg: command not found\n",
			"exitCode": 127, "durationMs": 31,
		},
	})
	f.wait()

	calls, listErr := f.activity.ToolCalls(f.ctx, chatID, 0, 0)
	require.NoError(t, listErr)
	require.Len(t, calls, 1, "tool_pre and tool_fail must correlate on item.id")
	assert.Equal(t, domain.ToolStatusError, calls[0].Status)
	// Without a target the transcript renders the bare word "commandExecution".
	assert.Equal(t, "rg --files", calls[0].Target)
}

// TestObservation_ACodexElicitationIsRecordedAsAnInterruptionAndAPrompt is
// codex's half of a9ebb6f1's promise ("elicitation... now answerable"),
// mirroring TestObservation_AnElicitationIsRecordedAsAnInterruptionAndAPrompt
// for claude. codex.yaml's elicitation: map only extracts message (schema:
// requestedSchema is the api-transport field name, with no hooks-shape
// fallback the way permission's tool_name: "tool || tool_name" has), so
// Title, Mode, and Schema all stay unset for a hooks-delivered elicitation —
// that is the actual declared mapping today, not an oversight this test
// should paper over.
func TestObservation_ACodexElicitationIsRecordedAsAnInterruptionAndAPrompt(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")

	hook(t, f, runnerID, "codex", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	hook(t, f, runnerID, "codex", engineagents.HookElicitation, map[string]any{
		"mcp_server_name": "spike", "message": "do you prefer A or B?",
	})

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1)
	assert.Equal(t, engineagents.InterruptElicitation, ints[0].Kind)

	got := pendingChoices(t, f, chatID)
	require.Len(t, got, 1)
	assert.Equal(t, domain.ChoiceKindElicitation, got[0].Kind)
	assert.Equal(t, "do you prefer A or B?", got[0].Question)
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

// TestRegression_InterruptedTurnGraduallyFinishingIsNotMisattributedToTheNextProvider
// is the exact bug reported live: Claude is asked to stop mid-message — a
// graceful request, not a kill (runner/lifecycle.go's StopChat falls through
// to TerminateGraceful for wire type "prompt") — the chat switches to Codex,
// Codex's OWN turn closes FIRST, and Claude's real text, still gracefully
// finishing, arrives LATE. Before the fix, closeAssistantTurn's
// t.messages.Open(chatID) was scoped only by chat, not by runner: Codex's
// turn_stop swept up Claude's still-open buffer and recorded Claude's own
// words under CODEX's provider. runnersMove constructs the window this
// needs (two runners placed on one chat — its own doc comment says this is
// exactly what it is for), and everything downstream of that — the user
// prompts, the deltas, the turn_stops — drives the real usecase, the real
// descriptor, and the real event-sourced aggregate, the same path
// production hits.
func TestRegression_InterruptedTurnGraduallyFinishingIsNotMisattributedToTheNextProvider(t *testing.T) {
	f := newFixture(t)

	chatID, claudeRunnerID := f.spawn(t, "claude")
	f.announce(t, claudeRunnerID, "s1")
	require.NoError(t, f.usecase.IngestHook(f.ctx, claudeRunnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "write a Valve essay"})))

	// Claude starts streaming its reply — NOT final: the user interrupts
	// (asks it to stop) before this message ever closes on its own.
	require.NoError(t, f.usecase.IngestHook(f.ctx, claudeRunnerID, "claude", "message_delta",
		deltaHook(t, "claude-msg", 0, false, "Valve Software: The Reluctant Giant")))
	f.wait()

	// The provider switch: a second runner (codex) placed on the SAME chat
	// while claude's runner still has it too — runnersMove constructs
	// exactly this window (its own doc comment: "two runners placed on one
	// chat"), matching the moment a real StopChat/displace has not yet won
	// its race against the outgoing CLI's own still-in-flight hook.
	_, codexRunnerID := f.spawn(t, "codex")
	require.NoError(t, f.runnersMove(t, codexRunnerID, chatID, "codex-s1"))

	// Codex, placed second, closes its OWN turn FIRST.
	codexPrompt := map[string]any{"prompt": "what did we talk before"}
	f.withTrackedSession(codexRunnerID, codexPrompt)
	require.NoError(t, f.usecase.IngestHook(f.ctx, codexRunnerID, "codex", "user_prompt", mustJSON(t, codexPrompt)))
	codexStop := map[string]any{"last_assistant_message": "codex reply"}
	f.withTrackedSession(codexRunnerID, codexStop)
	require.NoError(t, f.usecase.IngestHook(f.ctx, codexRunnerID, "codex", "turn_stop", mustJSON(t, codexStop)))
	f.wait()

	// Claude's real text — still gracefully finishing — arrives LATE, after
	// Codex has already closed its own turn on the same chat.
	require.NoError(t, f.usecase.IngestHook(f.ctx, claudeRunnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{
			"session_id":             "s1",
			"last_assistant_message": "Valve Software: The Reluctant Giant",
		})))
	f.wait()

	turns, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)

	var claudeTurn, codexTurn *domain.ActivityTurn
	for i := range turns {
		switch turns[i].Text {
		case "Valve Software: The Reluctant Giant":
			claudeTurn = &turns[i]
		case "codex reply":
			codexTurn = &turns[i]
		}
	}
	require.NotNil(t, claudeTurn, "claude's text was recorded at all")
	require.NotNil(t, codexTurn, "codex's text was recorded at all")

	assert.Equal(t, "claude", claudeTurn.ProviderID,
		"THE BUG: claude's own text must never be attributed to codex")
	assert.Equal(t, "codex", codexTurn.ProviderID)
	assert.Less(t, claudeTurn.DisplayOrder, codexTurn.DisplayOrder,
		"dispatched first, so displays first, even though it was recorded last")
}

// TestRegression_StopChatSalvagesTheAlreadyStreamedText is a live-reproduced bug
// (2026-08-30): stop a chat mid-stream — the exact "interrupt Claude, then send Codex
// something else" report — and the assistant's partial reply, already received and
// broadcast over message_delta, vanished entirely. closeAbandonedTurn (the function
// StopChat's retire()/displace() path funnels through) recorded only a bare "stopped"
// interruption marker and never looked at the streamed buffer at all, unlike the
// quiet-screen sweep's AbandonMessage, which does. Confirmed live against the real
// daemon and real claude CLI before this fix: the essay text visibly streamed to the
// client over the WS feed, then StopChat left NO assistant turn behind for it.
func TestRegression_StopChatSalvagesTheAlreadyStreamedText(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	// Named to match deltaHook's own session: a delta that names a conversation the
	// runner is not on is another one's, and ingest drops it.
	f.announce(t, runnerID, "sess-1")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "write a long essay about the transistor"})))

	// Claude is still streaming — NOT final — when the user hits Stop.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "message_delta",
		deltaHook(t, "essay-msg", 0, false, "The transistor stands as one of the most consequential inventions...")))
	f.wait()

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	turns, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)

	var salvaged *domain.ActivityTurn
	for i := range turns {
		if turns[i].Text == "The transistor stands as one of the most consequential inventions..." {
			salvaged = &turns[i]
		}
	}
	require.NotNil(t, salvaged,
		"THE BUG: text Crowbar already streamed to the client must survive a Stop, not disappear")
	assert.Equal(t, "claude", salvaged.ProviderID)
}

// TestRegression_ClearMidStreamSalvagesTheAlreadyStreamedText is the same gap as
// TestRegression_StopChatSalvagesTheAlreadyStreamedText, hit through a DIFFERENT door:
// moveToNewChat, reached whenever the live CLI itself announces a /clear (a session_start
// for a session id Crowbar doesn't recognise) while a message is still streaming on the
// chat it is LEAVING. closeAbandonedTurn is the same function both paths funnel through,
// so this is not a new bug — it is the same fix (AbandonMessageForRunner, threaded through
// every closeAbandonedTurn call site) proven on its other callers too, not just StopChat's.
func TestRegression_ClearMidStreamSalvagesTheAlreadyStreamedText(t *testing.T) {
	f := newFixture(t)

	chatA, runnerID := f.spawn(t, "claude")
	// Named to match deltaHook's own session, so the stream below really does land
	// on chat A — the turn moveToNewChat then has to salvage.
	f.announce(t, runnerID, "sess-1")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "write a long essay about the telegraph"})))

	// Claude is still streaming on chat A — NOT final — when the CLI itself /clears.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "message_delta",
		deltaHook(t, "clear-msg", 0, false, "The telegraph transformed long-distance communication...")))
	f.wait()

	f.announce(t, runnerID, "sB") // /clear — moveToNewChat mints chat B, abandoning A's turn

	moved := f.runner(t, runnerID)
	require.NotEqual(t, chatA, moved.CurrentChatID, "precondition: the runner really did move to a new chat")

	turns, err := f.activity.Turns(f.ctx, chatA, 0, 0, 0)
	require.NoError(t, err)

	var salvaged *domain.ActivityTurn
	for i := range turns {
		if turns[i].Text == "The telegraph transformed long-distance communication..." {
			salvaged = &turns[i]
		}
	}
	require.NotNil(t, salvaged,
		"the same salvage StopChat gets must also cover moveToNewChat's abandon")
	assert.Equal(t, "claude", salvaged.ProviderID)
}

// stopPayload is a real claude 2.1.212 Stop hook payload carrying `running` background
// tasks — the shape traced live while a background subagent was working. tasks is how
// many entries claude reports STILL OUTSTANDING as it goes quiet.
func stopPayload(t *testing.T, message string, tasks int) []byte {
	t.Helper()
	return stopPayloadFor(t, "s1", message, tasks)
}

// stopPayloadFor names the session the turn belongs to, for the tests whose
// fixture announces something other than "s1". A turn_stop that names a session
// the runner is not on is another conversation's, and ingest drops it — see
// namesAnotherConversation — so a payload's session has to match the announce
// that set the scene, exactly as a real CLI's would.
func stopPayloadFor(t *testing.T, session, message string, tasks int) []byte {
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
		"session_id":             session,
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
// background_tasks array leaves ev.AsyncWork at 0 — an unmapped field must never be
// counted. With no tool call or subagent open either (see
// TestRegression_CodexTurnStopWithOpenSubagent_KeepsChatWorking for that half), turn_stop
// is simply idle, same as before OpenWork's fallback existed.
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
	require.False(t, chat.Working, "codex maps no async_work and has no open tool/subagent: idle")
	require.Equal(t, 0, chat.AsyncWork, "an unmapped field must never be counted")
}

// TestRegression_CodexTurnStopWithOpenSubagent_KeepsChatWorking is the bug reported live:
// codex maps no async_work (TestCodexTurnStop_NeverReportsAsyncWork), so a subagent still
// running when codex's own top-level turn ends has nothing to restate it — the spinner
// went dark the instant codex sent its first mid-turn answer, even though the subagent it
// had just delegated to kept working. closeTurnFromStop's OpenWork fallback (the same
// generic tool/subagent open-close pairing OpenWork already answers termwait's idle check
// from) is what catches it here, since codex has no self-reported level like claude's
// background_tasks to restate it with (see stop_turn.go).
func TestRegression_CodexTurnStopWithOpenSubagent_KeepsChatWorking(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")
	prompt(t, f, runnerID, "codex", "investigate and delegate to a subagent")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "subagent_pre",
		mustJSON(t, map[string]any{
			"session_id": "sess-1", "agent_id": "sub-1", "agent_type": "explorer",
		})))
	f.wait()

	// codex's own top-level turn ends — with the subagent it just spawned still running.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "turn_stop",
		stopPayloadFor(t, "sess-1", "I'll delegate this to a subagent.", 0)))
	f.wait()

	chat := f.chat(t, chatID)
	require.True(t, chat.Working,
		"the spinner must KEEP SPINNING: codex's turn ended but its own subagent is still working")
	require.Nil(t, chat.CurrentTurnStarted, "the turn itself really did end")

	// The subagent finishes. Nothing else will ever restate this for codex — closing the
	// last piece of open work is what must clear it, or the fix would just trade "clears
	// too early" for "never clears".
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "subagent_post",
		mustJSON(t, map[string]any{
			"session_id": "sess-1", "agent_id": "sub-1", "agent_type": "explorer",
		})))
	f.wait()

	chat = f.chat(t, chatID)
	require.False(t, chat.Working, "once the subagent finishes the spinner MUST stop — no stuck-on")
}

// TestRegression_RestatingAsyncWorkDecidesOnTheLogNotTheProjection is the CHAT half of
// the same stuck-on spinner, and it is a property rather than a race: whatever the read
// model happens to be serving, the decision to restate must not come from it.
//
// turn_stop's StopTurn is on the async Send path, so the level it records is durable in
// the log before the projection folds it. A restate landing in that window — a subagent
// reporting back the instant its parent turn ended, which is the ordinary codex shape —
// reads AsyncWork as 0, finds the open-work level it just computed is 0 too, and returns
// as a no-op. Nothing else ever restates the level, so the aggregate stays lit forever.
// Live the window is microseconds wide; forcing it open is what turns "usually passes"
// into a property a test can hold. Same law as AbandonTurn's: callers must not pre-check
// turn state on the read model.
func TestRegression_RestatingAsyncWorkDecidesOnTheLogNotTheProjection(t *testing.T) {
	f, chats, _ := newFaultFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")
	prompt(t, f, runnerID, "codex", "investigate and delegate to a subagent")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "subagent_pre",
		mustJSON(t, map[string]any{
			"session_id": "sess-1", "agent_id": "sub-1", "agent_type": "explorer",
		})))
	f.wait()
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "turn_stop",
		stopPayloadFor(t, "sess-1", "I'll delegate this to a subagent.", 0)))
	f.wait()
	afterStop := f.chat(t, chatID)
	// Both halves, because Working alone is also true of a turn_stop that never
	// landed at all — the shape a payload naming the wrong session leaves behind.
	require.Nil(t, afterStop.CurrentTurnStarted,
		"precondition: the turn_stop actually closed the turn")
	require.True(t, afterStop.Working,
		"precondition: the turn ended with the subagent still running")

	// From here the read model serves the turn state from BEFORE that turn_stop —
	// the window the async Send path genuinely leaves open.
	chats.staleAsyncWorkProjection = true

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "subagent_post",
		mustJSON(t, map[string]any{
			"session_id": "sess-1", "agent_id": "sub-1", "agent_type": "explorer",
		})))
	f.wait()

	chat := f.chat(t, chatID)
	assert.False(t, chat.Working,
		"the last close must clear the spinner however far behind the projection is")
	assert.Equal(t, 0, chat.AsyncWork)
}

// TestRegression_TwoPiecesOfWorkClosingAtOnce_StopsTheSpinner is the same stuck-on
// spinner as its sibling above, reached the way a real CLI reaches it: a tool call and a
// subagent finishing at the SAME MOMENT, on two hook deliveries, on two goroutines. Every
// hook is its own net/http request, so nothing between the wire and the ingest orders two
// of them — a tool returning as a subagent reports back is an ordinary Tuesday, not a
// contrived interleaving.
//
// Both closes ask the same question ("is anything still open?") and the LAST answer wins
// the aggregate. The property is that whichever order they land in, the answer standing at
// the end is the true one: no work is open, so the spinner is off. A restate that decided
// on a level it read BEFORE its sibling's close — or on a projection that had not folded
// it — writes "still working" last and strands the chat lit forever.
func TestRegression_TwoPiecesOfWorkClosingAtOnce_StopsTheSpinner(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")
	prompt(t, f, runnerID, "codex", "run a command and delegate to a subagent")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "tool_pre",
		mustJSON(t, map[string]any{
			"session_id": "sess-1", "tool_use_id": "tool-1", "tool_name": "Bash",
			"tool_input": map[string]any{"command": "sleep 600"},
		})))
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "subagent_pre",
		mustJSON(t, map[string]any{
			"session_id": "sess-1", "agent_id": "sub-1", "agent_type": "explorer",
		})))
	f.wait()

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "turn_stop",
		stopPayloadFor(t, "sess-1", "Working on it.", 0)))
	f.wait()
	afterStop := f.chat(t, chatID)
	// Both halves, because Working alone is also true of a turn_stop that never
	// landed at all — the shape a payload naming the wrong session leaves behind.
	require.Nil(t, afterStop.CurrentTurnStarted,
		"precondition: the turn_stop actually closed the turn")
	require.True(t, afterStop.Working,
		"precondition: the turn ended with both the tool and the subagent still open")

	// Payloads are marshalled HERE, on the test's own goroutine: mustJSON reports
	// through require, and a require failure off the test goroutine is undefined.
	closes := []struct {
		kind    string
		payload []byte
	}{
		{"tool_post", mustJSON(t, map[string]any{
			"session_id": "sess-1", "tool_use_id": "tool-1", "tool_name": "Bash",
			"tool_input": map[string]any{"command": "sleep 600"}, "tool_response": "done",
		})},
		{"subagent_post", mustJSON(t, map[string]any{
			"session_id": "sess-1", "agent_id": "sub-1", "agent_type": "explorer",
		})},
	}
	// Released together, joined together: the two deliveries genuinely overlap rather
	// than being one-then-the-other with extra steps.
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, len(closes))
	for i, c := range closes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = f.usecase.IngestHook(f.ctx, runnerID, "codex", c.kind, c.payload)
		}()
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err, "ingesting %s", closes[i].kind)
	}
	f.wait()

	chat := f.chat(t, chatID)
	assert.False(t, chat.Working,
		"both pieces of work closed: whichever restate wrote last must have said so")
	assert.Equal(t, 0, chat.AsyncWork)
}

// TestRegression_CodexSubagentsDrainOneAtATime_SpinnerFollowsTheLastOne pins the
// read-after-write ordering restateAsyncWork depends on, in BOTH directions.
//
// subagent_post closes the subagent through the activity repo and then asks
// OpenWork — a SQL read of the very row it just closed — whether any subagent is
// still open. That write is projected by an asynx subscriber, so dispatching it
// without waiting for handlers let the read still see the subagent running: the
// recomputed level matched what the chat already held, restateAsyncWork returned
// early WITHOUT emitting the turn_stopped that clears Working, and since nothing
// re-runs that check the spinner stayed lit forever. Only codex shows it — claude
// restates its own async_work level on every turn_stop and so has a second
// mechanism that masks the stale read.
//
// Draining two subagents one at a time is what makes this a guard rather than a
// coincidence: the middle assertion fails for a "just always clear it" fix, and
// the last one fails for the stale-read bug.
func TestRegression_CodexSubagentsDrainOneAtATime_SpinnerFollowsTheLastOne(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")
	prompt(t, f, runnerID, "codex", "delegate this to two subagents")

	for _, id := range []string{"sub-1", "sub-2"} {
		require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "subagent_pre",
			mustJSON(t, map[string]any{
				"session_id": "sess-1", "agent_id": id, "agent_type": "explorer",
			})))
	}
	f.wait()

	// codex's own top-level turn ends with both subagents still running.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "turn_stop",
		stopPayloadFor(t, "sess-1", "Delegated to two subagents.", 0)))
	f.wait()
	require.True(t, f.chat(t, chatID).Working,
		"precondition: codex's turn ended but both its subagents are still working")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "subagent_post",
		mustJSON(t, map[string]any{
			"session_id": "sess-1", "agent_id": "sub-1", "agent_type": "explorer",
		})))
	f.wait()
	require.True(t, f.chat(t, chatID).Working,
		"one of the two finished — the spinner must KEEP SPINNING for the other")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "subagent_post",
		mustJSON(t, map[string]any{
			"session_id": "sess-1", "agent_id": "sub-2", "agent_type": "explorer",
		})))
	f.wait()
	require.False(t, f.chat(t, chatID).Working,
		"the last subagent finished — the spinner MUST stop, or the provider is done and the UI never says so")

	// The ledger the subagent shelf reads must agree with the spinner: both rows
	// closed, so the shelf shows nothing running either.
	subs, err := f.activity.Subagents(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, subs, 2)
	for _, s := range subs {
		assert.NotNil(t, s.EndedAt, "subagent %q must be closed in the ledger", s.ID)
	}
}

// TestRegression_CodexCompactionTurnNeverStopsTheChat is B1's own regression.
//
// Moving compact_pre/compact_post onto the live api transport (item/started /
// item/completed, item.type: contextCompaction) surfaced a trap only live
// capture found: thread/compact/start's response wraps that item pair in its
// OWN turn/started..turn/completed round trip, on the connection's SAME wire
// event turn_stop already consumes unconditionally. Confirmed live against
// codex-cli 0.149.1, that wrapper's turn/completed is byte-for-byte the same
// shape (items: [], itemsView: "notLoaded", status: "completed") a genuinely
// interrupted real turn produces — so nothing on the frame itself can tell a
// compaction's own close from an ordinary one. Without the turn_id-armed
// latch in turn/compaction.go, every compaction would append an inert-but-real
// turn_stopped event and reset the chat's turn bookkeeping.
func TestRegression_CodexCompactionTurnNeverStopsTheChat(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")

	before, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 100)
	require.NoError(t, err)
	lastActivityBefore := f.chat(t, chatID).LastActivityAt

	// compact_pre: the contextCompaction item/started, in the same shape
	// codex.yaml's own mapping reads (threadId, item.type, the envelope's
	// turnId) — this is what ARMS the latch. turn_id only resolves off
	// codex's api: channel block (the hooks: block maps only session_id), so
	// this and the wrapper turn_stop below must both be api-shaped on an
	// API-marked ctx.
	hookAPI(t, f, runnerID, "codex", "compact_pre", map[string]any{
		"threadId": "sess-1",
		"item":     map[string]any{"type": "contextCompaction", "id": "comp-item-1"},
		"turnId":   "compact-turn-1",
	})

	// The wrapper's own turn/completed — same turn id, no items — exactly the
	// shape captured live from a real thread/compact/start round trip.
	hookAPI(t, f, runnerID, "codex", "turn_stop", map[string]any{
		"threadId": "sess-1",
		"turn": map[string]any{
			"id":        "compact-turn-1",
			"items":     []any{},
			"itemsView": "notLoaded",
			"status":    "completed",
		},
	})

	chat := f.chat(t, chatID)
	require.False(t, chat.Working, "a compaction round trip must never mark the chat working")
	require.Nil(t, chat.CurrentTurnStarted, "no assistant turn was ever opened for this")
	// StopTurn.EmitEvent always stamps LastActivityAt to time.Now(), even when
	// nothing else about the projected state visibly changes (an idle chat's
	// Working and CurrentTurnStarted were already false/nil) — this is what
	// actually distinguishes "the guard skipped closeTurnFromStop entirely"
	// from "closeTurnFromStop ran and happened to restate the same values".
	require.Equal(t, lastActivityBefore, chat.LastActivityAt,
		"the compaction's own turn_stop must not touch the chat at all — StopTurn must never run for it")

	afterCompaction, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 100)
	require.NoError(t, err)
	require.Equal(t, len(before.Items), len(afterCompaction.Items),
		"the compaction's own turn_stop must record NOTHING in the ledger")

	// The guard must be surgically scoped to the armed id, not to turn_stop as
	// a whole: an ORDINARY turn right after, with its own different turn id,
	// must still record its reply exactly as it always has.
	prompt(t, f, runnerID, "codex", "what changed?")
	hookAPI(t, f, runnerID, "codex", "turn_stop", map[string]any{
		"threadId": "sess-1",
		"turn": map[string]any{
			"id":        "ordinary-turn-1",
			"items":     []any{map[string]any{"type": "agentMessage", "text": "nothing much"}},
			"itemsView": "summary",
			"status":    "completed",
		},
	})

	chat = f.chat(t, chatID)
	require.False(t, chat.Working, "the ordinary turn closed normally")

	afterOrdinary, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 100)
	require.NoError(t, err)
	require.Greater(t, len(afterOrdinary.Items), len(afterCompaction.Items),
		"an ordinary turn_stop with its OWN turn id must still record its reply — the guard must not be overbroad")
}

// TestRegression_CodexFailedCompactionTurnRecordsNoFailureNotice is the failure
// half of the sum type: turn_failed (not turn_stop) fires when
// thread/compact/start's wrapper turn ends with turn.status: failed. Without
// the same turn_id-armed guard in closeTurnFromFailure, this would append a
// spurious TurnRoleNotice row to the transcript ("failed: ...") for a turn
// that was never the assistant's own reply.
func TestRegression_CodexFailedCompactionTurnRecordsNoFailureNotice(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")

	before, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 100)
	require.NoError(t, err)

	// turn_failed is api-only (no hooks: block at all — same as tool_fail),
	// so both this and its own compact_pre below must be api-shaped on an
	// API-marked ctx.
	hookAPI(t, f, runnerID, "codex", "compact_pre", map[string]any{
		"threadId": "sess-1",
		"item":     map[string]any{"type": "contextCompaction", "id": "comp-item-2"},
		"turnId":   "compact-turn-2",
	})

	// The wrapper's turn/completed with turn.status: failed. IngestHook takes
	// the canonical event by name (not the raw wire frame), which bypasses
	// dispatch.Resolve's own when:-based turn_stop/turn_failed selection —
	// so this drives turn_failed directly, exactly as real dispatch would
	// have resolved this exact payload to, given the SAME turn id
	// compact_pre armed.
	hookAPI(t, f, runnerID, "codex", "turn_failed", map[string]any{
		"threadId": "sess-1",
		"turn": map[string]any{
			"id":     "compact-turn-2",
			"items":  []any{},
			"status": "failed",
			"error":  map[string]any{"message": "compaction blew up"},
		},
	})

	after, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 100)
	require.NoError(t, err)
	require.Equal(t, len(before.Items), len(after.Items),
		"a failed compaction round trip must record NO notice row in the transcript")
}

// TestRegression_CodexManualCompactionIsLabelledManual: codex's own
// contextCompaction item carries no trigger at all (see codex.yaml's
// compact_pre comment), so without Crowbar's own record of having just
// dispatched the request, every codex compaction — including one a person
// explicitly asked for via Compact() — read as "automatic" in the transcript
// divider. Compact() has no live api connection to actually send over in this
// fixture and returns an error, but ArmManualCompaction fires before that
// failure, exactly as it does in production.
//
// Drives compact_post too, not just compact_pre — live-confirmed critical:
// commands.Interrupt's idle branch never adds the interruption to the
// aggregate's own open-interruptions map, so commands.ResolveInterruption
// (compact_post's own call) always finds it unknown and REBUILDS it from
// scratch off compact_post's OWN (also-empty, for codex) detail, silently
// discarding whatever compact_pre just wrote. A version of this fix that
// only patched compact_pre passed this test right up until compact_post was
// added to it — caught live (a curl read briefly saw "manual", then read
// back empty about a second later) before it was caught here.
func TestRegression_CodexManualCompactionIsLabelledManual(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")

	_ = f.usecase.Compact(f.ctx, chatID)

	hookAPI(t, f, runnerID, "codex", "compact_pre", map[string]any{
		"threadId": "sess-1",
		"item":     map[string]any{"type": "contextCompaction", "id": "comp-item-3"},
		"turnId":   "compact-turn-3",
	})

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1)
	assert.Equal(t, "manual", ints[0].Detail, "compact_pre alone must already record manual")

	hookAPI(t, f, runnerID, "codex", "compact_post", map[string]any{
		"threadId": "sess-1",
		"item":     map[string]any{"type": "contextCompaction", "id": "comp-item-3"},
		"turnId":   "compact-turn-3",
	})

	ints, err = f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1)
	assert.Equal(t, "manual", ints[0].Detail,
		"compact_post must not rebuild the interruption and clobber manual back to empty")
}

// TestRegression_CodexAutomaticCompactionIsNotLabelledManual is the negative
// case the fix above must not break: a codex compaction NOBODY asked for
// through Compact() must keep reading as automatic (an empty Detail — the
// frontend's own fallback is what turns that into "automatically").
func TestRegression_CodexAutomaticCompactionIsNotLabelledManual(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")

	hookAPI(t, f, runnerID, "codex", "compact_pre", map[string]any{
		"threadId": "sess-1",
		"item":     map[string]any{"type": "contextCompaction", "id": "comp-item-4"},
		"turnId":   "compact-turn-4",
	})

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1)
	assert.Empty(t, ints[0].Detail)
}

// TestRegression_CodexAutoCompactionMidPromptDoesNotSettleTheRealDelivery is
// the live-reported bug: codex can decide, entirely on its own, to compact
// its context before it even starts processing a message a person already
// sent — the same compact_pre/compact_post pair a manual /compact produces,
// but this time a REAL prompt is what the daemon has pending, not the
// compaction (codex's own Compact() dispatch never touches the prompt
// journal at all — see settleCompactDelivery's own doc). Settling that real
// delivery made the just-submitted message vanish from the transcript for
// the whole compaction, reappearing only once the real turn finally opened.
func TestRegression_CodexAutoCompactionMidPromptDoesNotSettleTheRealDelivery(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "what changed?", uuid.NewString(), "", nil)
	require.NoError(t, err)
	require.True(t, agentusecase.HasPendingDelivery(f.usecase.RunnerUsecase, f.ctx, chatID),
		"precondition: the real prompt is dispatched and still unconfirmed")

	// SubmitPrompt's dispatch may have replaced the runner (this fixture has
	// no live api connection registered, so it falls to the restart_tui
	// path) — the delivery belongs to whichever runner is live NOW, exactly
	// as the pre-existing claude version of this fix's own tests already
	// account for.
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)

	// codex's OWN decision, never Compact() — the manual latch is deliberately
	// left unarmed, matching an automatic pre-turn compaction exactly.
	hookAPI(t, f, live.ID, "codex", "compact_pre", map[string]any{
		"threadId": "sess-1",
		"item":     map[string]any{"type": "contextCompaction", "id": "auto-comp-1"},
		"turnId":   "auto-compact-turn-1",
	})

	require.True(t, agentusecase.HasPendingDelivery(f.usecase.RunnerUsecase, f.ctx, chatID),
		"the real prompt must still be pending — compact_pre must not settle a delivery it did not itself create")

	hookAPI(t, f, live.ID, "codex", "compact_post", map[string]any{
		"threadId": "sess-1",
		"item":     map[string]any{"type": "contextCompaction", "id": "auto-comp-1"},
		"turnId":   "auto-compact-turn-1",
	})

	require.True(t, agentusecase.HasPendingDelivery(f.usecase.RunnerUsecase, f.ctx, chatID),
		"compact_post must not settle it either")
}
