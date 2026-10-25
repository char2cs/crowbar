package chat_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// ─── from observation_test.go ─────────────────────────────────────────

func hook(t *testing.T, f testFixture, runnerID, provider, kind string, payload map[string]any) {
	t.Helper()
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, provider, kind, mustJSON(t, payload)))
	f.wait()
}

// deadLetters is how many hooks the ingress has refused.
func deadLetters(t *testing.T, f testFixture) int {
	t.Helper()
	n, err := f.usecase.DeadLetterCount(f.ctx)
	require.NoError(t, err)
	return n
}

// refusedHook is a hook runnerID's descriptor cannot map: the ingress refuses it
// as unprocessable and records nothing of it but a marker.
func refusedHook(t *testing.T, f testFixture, runnerID, provider, kind string, payload map[string]any) {
	t.Helper()
	err := f.usecase.IngestHook(f.ctx, runnerID, provider, kind, mustJSON(t, payload))
	require.ErrorIs(t, err, apperr.ErrUnprocessable)
	f.wait()
}

// lateHook is a hook from a CLI that is no runner any more (exited, displaced, or
// its chat purged): the ingress refuses it as unknown and records nothing.
func lateHook(t *testing.T, f testFixture, runnerID, provider, kind string, payload map[string]any) {
	t.Helper()
	err := f.usecase.IngestHook(f.ctx, runnerID, provider, kind, mustJSON(t, payload))
	require.ErrorIs(t, err, apperr.ErrNotFound)
	f.wait()
}

// exchange is a whole turn on runnerID's CLI: the prompt that starts it on the
// chat's bound session, which makes that session resumable, then the reply.
func exchange(t *testing.T, f testFixture, runnerID, provider, reply string) {
	t.Helper()
	prompt(t, f, runnerID, provider, "go on")
	turn(t, f, runnerID, provider, reply)
}

// hookAPI is hook() placed on the api channel, as the runner's api connection
// places it. Use it for a payload shaped the way a provider's api: channel block
// maps it (codex's threadId/item.*/turn.* — see codex.yaml); through hook() it
// resolves against the hooks block and is misparsed or refused.
func hookAPI(t *testing.T, f testFixture, runnerID, provider, kind string, payload map[string]any) {
	t.Helper()
	_, err := f.usecase.placeHook(f.ctx, engineagents.ChannelAPI, runnerID, provider, kind, mustJSON(t, payload))
	require.NoError(t, err)
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

func TestRegression_EditToolCallCarriesADiffOfItsOwnChange(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt,
		map[string]any{"prompt": "edit the file"})
	hook(t, f, runnerID, "claude", engineagents.HookToolPre, map[string]any{
		"tool_use_id": "tool-1", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": "a.go", "old_string": "x\n", "new_string": "y\n"},
	})
	hook(t, f, runnerID, "claude", engineagents.HookToolPost, map[string]any{
		"tool_use_id": "tool-1", "tool_name": "Edit",
		"tool_input":    map[string]any{"file_path": "a.go"},
		"tool_response": "applied",
	})

	calls, err := f.activity.ToolCalls(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.Equal(t, "--- a/a.go\n+++ b/a.go\n@@ -1,1 +1,1 @@\n-x\n+y\n", calls[0].Diff)
}

func TestRegression_ACompletedEditReplacesItsSnippetDiffWithTheProvidersTrueHunks(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt,
		map[string]any{"prompt": "edit the file"})
	hook(t, f, runnerID, "claude", engineagents.HookToolPre, map[string]any{
		"tool_use_id": "tool-1", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": "/repo/a.go", "old_string": "x\n", "new_string": "y\n"},
	})
	hook(t, f, runnerID, "claude", engineagents.HookToolPost, map[string]any{
		"tool_use_id": "tool-1", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": "/repo/a.go"},
		"tool_response": map[string]any{"structuredPatch": []any{map[string]any{
			"oldStart": 8, "oldLines": 3, "newStart": 8, "newLines": 3,
			"lines": []any{" before", "-x", "+y", " after"},
		}}},
	})

	calls, err := f.activity.ToolCalls(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.Equal(t, "--- a/repo/a.go\n+++ b/repo/a.go\n@@ -8,3 +8,3 @@\n before\n-x\n+y\n after\n", calls[0].Diff)
}

// The hook's text names the attachment by its absolute store path; the ledger
// keeps the logical chats/<chatID>/attachments/<file> ref the asset endpoint serves.
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

func TestObservation_AnonymousSubagentStopsDoNotCreatePhantomRows(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	refusedHook(t, f, runnerID, "claude", engineagents.HookSubagentPost, map[string]any{"agent_type": ""})
	refusedHook(t, f, runnerID, "claude", engineagents.HookSubagentPost, map[string]any{"agent_type": ""})

	subs, err := f.activity.Subagents(f.ctx, chatID)
	require.NoError(t, err)
	assert.Empty(t, subs)
}

// A spawned agent's child thread, opened by its spawn's completion, records its
// own tool calls and reply under its scope: never dropped, never the chat's turn.
func TestObservation_ANestedSubagentsToolCallsAndReplyAreRecorded(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "thread-main")
	hook(t, f, runnerID, "codex", "user_prompt", map[string]any{"prompt": "delegate"})

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
	require.Len(t, turns, 1, "only the prompt: the child's reply is the subagent's")
}

// TestRegression_SpawnAgentCompletionOpensTheNestedSubagent is the bug
// live-reported against SubagentShelf: a codex chat ran a real collab_agents
// subagent, and the live shelf never showed it running at all.
func TestRegression_SpawnAgentCompletionOpensTheNestedSubagent(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "thread-main")

	hook(t, f, runnerID, "codex", "user_prompt", map[string]any{"prompt": "spawn a subagent"})
	require.True(t, f.busy(t, chatID), "precondition: the turn is open")

	// codex ends its OWN visible turn the instant it delegates — nothing else
	// is open once this lands.
	hook(t, f, runnerID, "codex", "turn_stop", map[string]any{
		"session_id": "thread-main", "last_assistant_message": "Delegating now.",
	})
	require.False(t, f.busy(t, chatID),
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

	subs, err := f.activity.Subagents(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	assert.Nil(t, subs[0].EndedAt, "opened, not yet closed")
}

// A resumed codex chat can have no bound conversation (thread/resume fires no
// thread/started), and its subagents' prompts and replies must still stay out of
// the main transcript rather than pass as the user's and the agent's own.
func TestRegression_ASubagentsPromptAndReplyNeverEnterTheMainTranscript(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "thread-main")
	exchange(t, f, runnerID, "codex", "answered before the TUI stopped")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.term.exit(t, "term-1")
	f.wait()
	resumedID, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	require.Contains(t, f.term.calls[f.term.callCount()-1].argv, "thread-main",
		"precondition: the revive resumes the chat's own thread")

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
	assert.Contains(t, kinds, ledger.MarkerNotification)
	assert.Contains(t, kinds, engineagents.HookPermission)
}

func TestObservation_ACompactionOpensAndResolvesTheSameRecord(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookCompactPre, map[string]any{"trigger": "auto"})
	hook(t, f, runnerID, "claude", engineagents.HookCompactPost, map[string]any{"trigger": "auto"})

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1)
	assert.Equal(t, "compaction", ints[0].Kind)
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
	f.usecase.StartTerminalWaitSweep(f.ctx, agentusecase.ChatFeed{
		Compaction: func(_, _ string, active bool) {
			mu.Lock()
			defer mu.Unlock()
			calls = append(calls, active)
		},
	})

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

func TestObservation_AnUndeclaredEventIsRefusedNotRecorded(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")

	refusedHook(t, f, runnerID, "codex", engineagents.HookNotification,
		map[string]any{"transcript_path": "/rollouts/x", "message": "hi"})

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

// §6a: the gauge is pushed, not polled. Every report the chat's surface
// carries is published on the chat feed the moment it lands.
func TestTelemetry_IsPushedOnTheChatFeed(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	var mu sync.Mutex
	var pushed []engineagents.Telemetry
	f.usecase.StartTerminalWaitSweep(f.ctx, agentusecase.ChatFeed{
		Telemetry: func(id, _ string, report engineagents.Telemetry) {
			mu.Lock()
			defer mu.Unlock()
			if id == chatID {
				pushed = append(pushed, report)
			}
		},
	})

	hook(t, f, runnerID, "claude", engineagents.HookTelemetry, map[string]any{
		"context_window": map[string]any{"context_window_size": 200000, "used_percentage": 19},
	})

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, pushed, 1)
	require.NotNil(t, pushed[0].Context)
	assert.InDelta(t, 19, *pushed[0].Context.UsedPercent, 0.001)
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

// A tool_pre with no tool id is a malformed delivery that required: rejects:
// a minted fallback id would give one call's two deliveries two ids.
func TestObservation_AToolPreWithNoIDIsRejectedNotRecorded(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	refusedHook(t, f, runnerID, "claude", engineagents.HookToolPre, map[string]any{"tool_name": "Bash"})

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

// An observation the record refuses is refused to its sender and kept as a dead
// letter: a hook is never acknowledged with nothing written.
func TestObservation_AnUnrecordableObservationIsRefusedAndDeadLettered(t *testing.T) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")
	f.refuseRows(t, ledger.KindToolCall, ledger.KindSubagent, ledger.KindMarker, ledger.KindAsk)
	letters := deadLetters(t, f)

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
	} {
		err := f.usecase.IngestHook(f.ctx, runnerID, "claude", tc.kind, mustJSON(t, tc.payload))
		require.ErrorContains(t, err, "log is full", tc.kind)
		letters++
		assert.Equal(t, letters, deadLetters(t, f), tc.kind)
	}
}

func TestObservation_AHookFromARunnerPlacedNowhereIsRefused(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	err := f.usecase.IngestHook(f.ctx, runnerID, "claude", engineagents.HookToolPre,
		mustJSON(t, map[string]any{"tool_use_id": "t1", "tool_name": "Bash"}))

	assert.ErrorIs(t, err, apperr.ErrNotFound)
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
	require.ErrorIs(t, err, agentusecase.ErrToolPayloadGone)

	_, err = f.usecase.ReadToolPayload(f.ctx, ownerChat, "t1", "request")
	assert.NoError(t, err)
}

func TestReadToolPayload_MissingToolOrSideIsNotFound(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	hook(t, f, runnerID, "claude", engineagents.HookToolPre,
		map[string]any{"tool_use_id": "t1", "tool_name": "Bash"})

	_, err := f.usecase.ReadToolPayload(f.ctx, chatID, "no-such-tool", "request")
	require.ErrorIs(t, err, agentusecase.ErrToolPayloadGone)

	_, err = f.usecase.ReadToolPayload(f.ctx, chatID, "t1", "request")
	assert.ErrorIs(t, err, agentusecase.ErrToolPayloadGone)
}

func TestReadToolPayload_RefusesAChatThatDoesNotExist(t *testing.T) {
	f := newFixture(t)

	_, err := f.usecase.ReadToolPayload(f.ctx, "no-such-chat", "t1", "request")

	require.Error(t, err)
}

// ─── from hook_delivery_test.go ───────────────────────────────────────

// Two hooks with the same words are two turns: a hook is never taken for a
// retry of the last one.
func TestHook_TwoIdenticalTurnEndsAreTwoTurns(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	payload := mustJSON(t, map[string]any{"last_assistant_message": "same words"})

	for range 2 {
		require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop", payload))
	}
	f.wait()

	turns, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	assert.Len(t, turns, 2)
}

// A hook from a runner the daemon does not hold is refused and kept as a dead
// letter, so a CLI that outlived its runner is visible rather than silent.
func TestHook_AnUnknownRunnerIsRefusedAndDeadLettered(t *testing.T) {
	f := newFixture(t)

	lateHook(t, f, uuid.NewString(), "claude", "session_start", map[string]any{"session_id": "sid-1"})

	assert.Empty(t, f.bc.snapshot())
	assert.Empty(t, f.runnerKinds(t), "a hook must never resurrect a runner we do not know")
	assert.Equal(t, 1, deadLetters(t, f))
}

// Once the PTY is gone no turn opens on its runner: a late prompt from it
// changes nothing. (Liveness has exactly one source: the process.)
func TestHook_AnExitedRunnersPromptOpensNothing(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()
	f.bc.reset()

	hook(t, f, runnerID, "claude", "user_prompt", map[string]any{"prompt": "a ghost speaks"})

	assert.Empty(t, f.bcKinds(t))
	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)
	assert.Empty(t, handoff, "a dead runner's hook must not write to the ledger")
}

// An event the descriptor does not declare is refused, and the chat's transcript
// marks where it was lost.
func TestHook_AnUnmappedEventIsRefusedWithAMarker(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	refusedHook(t, f, runnerID, "claude", "not_a_real_hook", map[string]any{})

	markers := f.markers(t, chatID)
	require.Len(t, markers, 1)
	assert.True(t, strings.HasPrefix(markers[0], ledger.MarkerDeadLetter+":not_a_real_hook: "), markers[0])
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

func TestRegression_ATurnEndingNoticeFailsTheTurnAndTheChatSaysWhy(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")
	prompt(t, f, runnerID, "codex", "please do the thing")
	require.True(t, f.busy(t, chatID), "the user's prompt must have opened a turn")

	notice, ok := f.usecase.MatchTerminalNotice(f.ctx, "codex", codexUsageLimitScreen)
	require.True(t, ok, "the shipped codex descriptor must recognise its own usage-limit banner")

	agentusecase.FailTurnOnNotice(f.ctx, f.usecase.TurnUsecase, f.runner(t, runnerID), notice)
	f.wait()

	assert.False(t, f.busy(t, chatID), "the wedged spinner must stop")

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

// A notice ending the turn keeps the reply already streamed live: codex
// streams part of a reply, then hits its usage limit before its own
// turn/completed ever arrives.
func TestRegression_ATurnEndingNoticeSalvagesTheAlreadyStreamedText(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")
	prompt(t, f, runnerID, "codex", "please do the thing")
	require.True(t, f.busy(t, chatID), "the user's prompt must have opened a turn")

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
	agentusecase.FailTurnOnNotice(f.ctx, f.usecase.TurnUsecase, f.runner(t, runnerID), notice)
	f.wait()

	assert.False(t, f.busy(t, chatID), "the wedged spinner must stop")

	turns, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	var salvaged *domain.ActivityTurn
	for i := range turns {
		if turns[i].Text == "Here is the first part of the answer..." {
			salvaged = &turns[i]
		}
	}
	require.NotNil(t, salvaged,
		"text Crowbar already streamed to the client must survive the notice, not disappear")
	assert.Equal(t, "codex", salvaged.ProviderID)
}

func TestUsecase_FailTurnOnNotice_WritesNoNoticeWhenThereWasNoTurnToClose(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")
	require.False(t, f.busy(t, chatID), "no prompt was sent: there is no open turn")

	notice, ok := f.usecase.MatchTerminalNotice(f.ctx, "codex", codexUsageLimitScreen)
	require.True(t, ok)
	agentusecase.FailTurnOnNotice(f.ctx, f.usecase.TurnUsecase, f.runner(t, runnerID), notice)
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

func TestRegression_ANoticeIsDurableBeforeTheTurnFails(t *testing.T) {
	log := &orderLog{}
	f := newFixture(t)
	f.ledger.Subscribe(func(_ string, rows []ledger.Row, _ ledger.State) {
		for _, r := range rows {
			if body, err := ledger.DecodeBody[ledger.TurnFailed](r); err == nil && r.Kind == ledger.KindTurnFailed {
				log.note("turn-failed: " + body.Detail)
			}
		}
	})
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")
	prompt(t, f, runnerID, "codex", "please do the thing")
	require.True(t, f.busy(t, chatID))

	notice, ok := f.usecase.MatchTerminalNotice(f.ctx, "codex", codexUsageLimitScreen)
	require.True(t, ok)
	agentusecase.FailTurnOnNotice(f.ctx, f.usecase.TurnUsecase, f.runner(t, runnerID), notice)
	f.wait()

	assert.Equal(t, []string{"turn-failed: " + notice.Text}, log.all(),
		"the explanation is durable in the very row that ends the turn")
}

// ─── from choice_test.go ──────────────────────────────────────────────

func permissionPayload() map[string]any {
	return map[string]any{
		"prompt_id": "81899da5", "permission_mode": "default",
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
		"tool_input": permissionPayload()["tool_input"],
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

// Once its turn ended, only the background work that end declared can hold the
// CLI on an ask: held, it stays a card to answer (held ⇔ answerable).
func TestObservation_AHeldAskOutlivesAnEndThatDeclaredBackgroundWork(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	prompt(t, f, runnerID, "claude", "go")
	hook(t, f, runnerID, "claude", engineagents.HookSubagentPre, map[string]any{"agent_id": "a1", "agent_type": "general-purpose"})
	ask := deliver(t, f, runnerID, "claude", engineagents.HookPermission, permissionPayload())
	require.NotEmpty(t, ask)

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop", stopPayloadFor(t, "s1", "main done", 1)))
	f.wait()

	_, held := f.usecase.PendingAnswer(ask)
	require.True(t, held, "precondition: the desk still holds the CLI")
	assert.Len(t, pendingChoices(t, f, chatID), 1, "held ⇔ answerable")
	st, err := f.ledger.State(f.ctx, chatID)
	require.NoError(t, err)
	assert.Len(t, st.PendingAsks(), 1, "the chat's frames announce it too")
}

// Its CLI blocks on a held ask whether or not a turn runs: it is there to answer.
func TestObservation_APermissionWithNoTurnOpenIsPending(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookPermission, permissionPayload())

	assert.Len(t, pendingChoices(t, f, chatID), 1)
}

func TestObservation_AskUserQuestionIsRecordedWithItsLabelledOptions(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")

	hook(t, f, runnerID, "claude", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})
	hook(t, f, runnerID, "claude", engineagents.HookPermission, map[string]any{
		"prompt_id": "q1", "tool_name": "AskUserQuestion",
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
	assert.Equal(t, engineagents.HookElicitation, ints[0].Kind)

	got := pendingChoices(t, f, chatID)
	require.Len(t, got, 1)
	assert.Equal(t, domain.ChoiceKindElicitation, got[0].Kind)
	assert.Equal(t, "spike", got[0].Title)
	assert.Equal(t, "do you prefer A or B?", got[0].Question)
	assert.Equal(t, "form", got[0].Mode)
	assert.Contains(t, got[0].Schema, `"enum":["A","B"]`)
}

// It proves an unmapped kind is dropped, never failed, so it must name a kind
// codex genuinely does not declare: notification is what is left.
func TestObservation_ACodexChatObservesNoNotification(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	hook(t, f, runnerID, "codex", engineagents.HookUserPrompt, map[string]any{"prompt": "go"})

	refusedHook(t, f, runnerID, "codex", engineagents.HookNotification,
		map[string]any{"message": "needs your attention"})

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
	assert.Equal(t, engineagents.HookElicitation, ints[0].Kind)

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
		"tool_name":  "shell",
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

func TestReadPendingChoices_RefusesAChatThatDoesNotExist(t *testing.T) {
	f := newFixture(t)

	_, err := f.usecase.ReadPendingChoices(f.ctx, "no-such-chat")

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

func TestRegression_PromptFiredInsideASubagentIsNeverRecordedAsTheUsers(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{
			"prompt": "Summarize the failing tests", "agent_id": "aa3b60603214670cc",
			"agent_type": "general-purpose",
		})))
	f.wait()

	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, page.Items, "a subagent's prompt is not something the user said")
	assert.False(t, f.busy(t, chatID), "a subagent's prompt opens no top-level turn")
}

func TestRegression_TeammateMessageIsRecordedAsHarnessNotUser(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	prompt := `<teammate-message teammate_id="reviewer" summary="done">Found 2 issues</teammate-message>`

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": prompt})))
	f.wait()

	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, domain.TurnRoleHarness, page.Items[0].Role)
}

func TestRegression_SubagentHandBackIsRecordedAsHarnessNotUser(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	prompt := "[Subagent hand-back] The text below is the final report of a subagent this session delegated to.\n  Done."

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": prompt})))
	f.wait()

	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, domain.TurnRoleHarness, page.Items[0].Role)
}

// A user_prompt hook starts the turn for the Crowbar prompt only when its text is
// that prompt's; text typed at the CLI or written by its harness starts its own
// turn and leaves the Crowbar prompt pending.
func TestUserPromptHook_AcknowledgesOnlyTheCrowbarPromptWhoseTextItCarries(t *testing.T) {
	for _, tc := range []struct{ name, text, origin string }{
		{"typed at the CLI", "something typed at the CLI", ledger.OriginProvider},
		{"the provider's harness", taskNotificationPrompt, ledger.OriginHarness},
		{"the prompt itself", "reply with only: SEED", ledger.OriginCrowbar},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			chatID, runnerID := f.spawn(t, "claude")
			f.announce(t, runnerID, "sid-claude-native")
			requestID := uuid.NewString()
			_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "reply with only: SEED", requestID, "", nil)
			require.NoError(t, err)
			live, err := f.liveRunnerFor(t, chatID)
			require.NoError(t, err)
			before := len(ledgerRows(t, f, chatID))

			prompt(t, f, live.ID, "claude", tc.text)

			rows, got := ledgerRows(t, f, chatID)[before:], receipt(t, f, chatID, requestID)
			if tc.origin == ledger.OriginCrowbar {
				require.Len(t, rows, 1)
				assert.Equal(t, ledger.KindTurnStarted, rows[0].Kind)
				assert.Equal(t, got.TurnID, rows[0].TurnID, "the turn starts under the prompt's own TurnID")
				assert.Equal(t, ledger.PromptStatusDelivered, got.Status)
				return
			}
			require.Len(t, rows, 2)
			msg, err := ledger.DecodeBody[ledger.UserMessage](rows[0])
			require.NoError(t, err)
			assert.Equal(t, tc.origin, msg.Origin)
			assert.Equal(t, []any{ledger.KindTurnStarted, rows[0].TurnID}, []any{rows[1].Kind, rows[1].TurnID})
			assert.Equal(t, ledger.PromptStatusPending, got.Status, "nothing Crowbar queued was taken")
		})
	}
}

func TestRegression_FinalIncrementAheadOfAnEarlierOneNeverRecordsATruncatedMessage(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "greet me"})))

	// Hooks are separate processes, so the closing increment can land first.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "message_delta",
		deltaHook(t, "m1", 1, true, "world")))
	f.wait()

	turns, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	for _, turn := range turns {
		assert.NotEqual(t, "world", turn.Text, "a message with a missing increment must not be recorded yet")
	}

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "message_delta",
		deltaHook(t, "m1", 0, false, "Hello ")))
	f.wait()

	turns, err = f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	var recorded []string
	for _, turn := range turns {
		if turn.Role == domain.TurnRoleAssistant {
			recorded = append(recorded, turn.Text)
		}
	}
	assert.Equal(t, []string{"Hello world"}, recorded)
}

func TestRegression_HarnessInjectedPromptStillOpensTheTurn(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.False(t, f.busy(t, chatID), "a fresh chat is not busy")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": taskNotificationPrompt})))

	assert.True(t, f.busy(t, chatID), "an injected prompt still opens the turn")
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

// Work a turn_stop reports still running keeps the chat busy, so a switch is
// refused until a later turn's end restates the level at zero.
func TestSwitchProvider_BackgroundWork_IsRefusedUntilItEnds(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	oldTerm := f.runner(t, runnerID).TerminalSession
	f.announce(t, runnerID, "s1")
	prompt(t, f, runnerID, "claude", "launch a background subagent")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "Launched.", 1)))
	f.wait()
	require.True(t, f.busy(t, chatID), "precondition: async work keeps the chat live")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.ErrorIs(t, err, agentusecase.ErrPromptBusy)
	require.Empty(t, f.term.terminatedIDs(), "background work must keep the outgoing TUI alive")

	prompt(t, f, runnerID, "claude", taskNotificationPrompt)
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "The background subagent finished.", 0)))
	f.wait()

	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	require.Contains(t, f.term.terminatedIDs(), oldTerm)
}

// A stopped Claude still finishing its text after a switch to Codex: what it
// streamed is kept under Claude when it exits, and its late turn_stop lands in
// its own turn, so nothing of it lands under Codex.
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

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	codexRunnerID, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.announce(t, codexRunnerID, "codex-s1")

	codexPrompt := map[string]any{"prompt": "what did we talk before"}
	require.NoError(t, f.usecase.IngestHook(f.ctx, codexRunnerID, "codex", "user_prompt", mustJSON(t, codexPrompt)))
	codexStop := map[string]any{"last_assistant_message": "codex reply"}
	require.NoError(t, f.usecase.IngestHook(f.ctx, codexRunnerID, "codex", "turn_stop", mustJSON(t, codexStop)))
	f.wait()

	// Claude's text — still gracefully finishing — arrives LATE, after Codex
	// has already closed its own turn on the same chat: its runner is gone.
	err = f.usecase.IngestHook(f.ctx, claudeRunnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{
			"session_id":             "s1",
			"last_assistant_message": "Valve Software: The Reluctant Giant",
		}))
	require.NoError(t, err)
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
// TestRegression_StopChatSalvagesTheAlreadyStreamedText through another door: the CLI
// /clears onto a new session while a message is still streaming on the old one.
func TestRegression_ClearMidStreamSalvagesTheAlreadyStreamedText(t *testing.T) {
	f := newFixture(t)

	chatA, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "write a long essay about the telegraph"})))

	// Claude is still streaming — NOT final — when the CLI itself /clears.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "message_delta",
		deltaHook(t, "clear-msg", 0, false, "The telegraph transformed long-distance communication...")))
	f.wait()

	f.announce(t, runnerID, "sB")
	f.wait()

	require.Equal(t, chatA, f.runner(t, runnerID).CurrentChatID, "precondition: a /clear keeps the runner on its chat")

	turns, err := f.activity.Turns(f.ctx, chatA, 0, 0, 0)
	require.NoError(t, err)

	var salvaged *domain.ActivityTurn
	for i := range turns {
		if turns[i].Text == "The telegraph transformed long-distance communication..." {
			salvaged = &turns[i]
		}
	}
	require.NotNil(t, salvaged,
		"the same salvage StopChat gets must also cover the session a /clear left")
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

// Claude fires Stop while a background subagent still works; the async-work level
// on that Stop must travel from the hook payload into the fold so the chat stays
// working.
func TestRegression_TurnStopWithBackgroundSubagent_KeepsChatWorking(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "launch a background subagent and wait for it"})))
	require.True(t, f.busy(t, chatID), "precondition: the turn is open")

	// claude hands the work off and ends its turn — with one subagent still running.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "Launched. The subagent is running in the background.", 1)))
	f.wait()

	require.True(t, f.busy(t, chatID),
		"the spinner must KEEP SPINNING: the turn ended but a background subagent is still working")
	require.False(t, running(t, f, chatID), "the turn itself really did end")
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
	require.True(t, f.busy(t, chatID), "precondition: spinning on the subagent")

	// The subagent reports back: claude re-invokes itself and ends the turn with nothing left.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "<task-notification><task-id>abbe4333c2384e2dc</task-id>"})))
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "The subagent finished.", 0)))
	f.wait()

	require.False(t, f.busy(t, chatID), "once the work is done the spinner MUST stop — no stuck-on")
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
			require.Truef(t, f.busy(t, chatID),
				"%d subagents still running: must keep spinning", outstanding)
		}
	}

	require.False(t, f.busy(t, chatID), "drained to zero: the spinner must stop")
}

// An interrupt fires no hook, so the CLI never closes the turn it cut: the
// next prompt's turn supersedes it, and that turn's own Stop settles the level.
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
	require.True(t, f.busy(t, chatID), "precondition: spinning on 3 subagents")

	// ...the user hits ESC. NOTHING arrives — that is the whole point; no hook exists.
	// Then they type again. This is the only edge that can heal it.
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "hi"})))
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		stopPayload(t, "hello", 0)))
	f.wait()

	require.False(t, f.busy(t, chatID),
		"an interrupt must not strand the spinner: the next completed turn settles it")
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
	require.True(t, f.busy(t, chatID), "precondition: spinning on announced work")
	require.False(t, running(t, f, chatID),
		"and NOT because a turn is open — the turn closed; only the work keeps it lit")

	// The CLI dies with the daemon. No Stop is coming, ever.
	f.term.dieWithDaemon()
	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	require.False(t, f.busy(t, chatID),
		"a dead CLI's announced work must not spin the chat forever across a restart")
}

// TestCodexTurnStop_NeverReportsAsyncWork is the PROVIDER-AGNOSTIC requirement through the
// live usecase: codex maps no async_work, so even a Stop payload that happens to carry a
// background_tasks array leaves ev.AsyncWork at 0 — an unmapped field must never be
// counted.
func TestCodexTurnStop_NeverReportsAsyncWork(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "s1")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "do a thing"})))
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "codex", "turn_stop",
		stopPayload(t, "done", 3)))
	f.wait()

	require.False(t, f.busy(t, chatID), "codex maps no async_work: an unmapped field must never be counted")
}

// thread/compact/start wraps its item pair in its own turn/started..completed,
// byte-for-byte an ordinary turn's close; only the turn_id-armed latch keeps a
// compaction from stopping the chat's turn.
func TestRegression_CodexCompactionTurnNeverStopsTheChat(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")

	before, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 100)
	require.NoError(t, err)
	rowsBefore, err := f.ledger.Read(f.ctx, chatID, 0, 0)
	require.NoError(t, err)

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

	require.False(t, f.busy(t, chatID), "a compaction round trip must never mark the chat working")
	rowsAfter, err := f.ledger.Read(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	for _, r := range rowsAfter[len(rowsBefore):] {
		require.Equal(t, ledger.KindMarker, r.Kind, "the compaction's own turn_stop must not touch the chat's turns at all")
	}

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

	require.False(t, f.busy(t, chatID), "the ordinary turn closed normally")

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

// Codex's compaction item carries no trigger, so Crowbar's own record of the
// Compact() it dispatched labels the divider manual. compact_post is driven too:
// it rebuilds the interruption and must not discard what compact_pre wrote.
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
// compaction (codex's own Compact() dispatch never records a prompt
// at all — see settleCompactDelivery's own doc). Settling that real
// delivery made the just-submitted message vanish from the transcript for
// the whole compaction, reappearing only once the real turn finally opened.
func TestRegression_CodexAutoCompactionMidPromptDoesNotSettleTheRealDelivery(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "what changed?", uuid.NewString(), "", nil)
	require.NoError(t, err)
	require.True(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID),
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

	require.True(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID),
		"the real prompt must still be pending — compact_pre must not settle a delivery it did not itself create")

	hookAPI(t, f, live.ID, "codex", "compact_post", map[string]any{
		"threadId": "sess-1",
		"item":     map[string]any{"type": "contextCompaction", "id": "auto-comp-1"},
		"turnId":   "auto-compact-turn-1",
	})

	require.True(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID),
		"compact_post must not settle it either")
}

// The user turn a Crowbar-dispatched prompt produces carries that dispatch's
// own request id, so a client confirms its queued prompt by identity rather
// than by comparing text the CLI may have reshaped.
func TestUserTurn_ADispatchedPromptIsRecordedUnderItsRequestID(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-1")
	exchange(t, f, runnerID, "claude", "an earlier answer")
	requestID := uuid.NewString()

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "the dispatched prompt", requestID, "", nil)
	require.NoError(t, err)
	f.wait()
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	require.NoError(t, f.usecase.IngestHook(f.ctx, live.ID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "the dispatched prompt", "session_id": "sid-1"})))
	f.wait()

	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 100)
	require.NoError(t, err)
	var ids []string
	for _, item := range page.Items {
		if item.Role == "user" && item.Text == "the dispatched prompt" {
			ids = append(ids, item.ID)
		}
	}
	assert.Equal(t, []string{requestID}, ids)
}

// claudeHookNames maps the provider's own hook names, as they appear in a
// captured payload's hook_event_name, to the canonical events the ingest takes.
var claudeHookNames = map[string]string{
	"SessionStart": "session_start", "UserPromptSubmit": "user_prompt", "Stop": "turn_stop",
	"MessageDisplay": "message_delta", "PreToolUse": "tool_pre", "PostToolUse": "tool_post",
	"PostToolUseFailure": "tool_fail", "SubagentStart": "subagent_pre", "SubagentStop": "subagent_post",
	"PreCompact": "compact_pre", "PostCompact": "compact_post", "SessionEnd": "session_end",
}

// replayRealClaudeSession feeds a sanitized capture of a REAL claude session
// (two background subagents that call tools, their hand-backs, /compact, a
// follow-up) through IngestHook in recorded order.
func replayRealClaudeSession(t *testing.T, f testFixture, runnerID string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "..", "..", "tests", "testdata", "hooks", "claude-subagent-session", "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for i, path := range files {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		var p struct {
			Event   string `json:"hook_event_name"`
			Session string `json:"session_id"`
		}
		require.NoError(t, json.Unmarshal(raw, &p))
		if i == 0 {
			f.announce(t, runnerID, p.Session)
		}
		require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", claudeHookNames[p.Event], raw))
		f.wait()
	}
}

func TestRegression_RealClaudeSubagentSession_OnlyTheHumansWordsAreUserRows(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	replayRealClaudeSession(t, f, runnerID)

	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	var users []string
	harness := 0
	for _, m := range page.Items {
		switch m.Role {
		case domain.TurnRoleUser:
			users = append(users, m.Text)
		case domain.TurnRoleHarness:
			harness++
		}
	}
	assert.Equal(t, []string{
		"Launch two subagents in the background with the Agent tool (general-purpose), each must Read a.txt then run Bash 'echo hi'. Report each in one line. Then say DONE.",
		"what is 2+2? one word",
	}, users, "only what the human typed may be a user row")
	assert.Equal(t, 2, harness, "each subagent hand-back is kept, as the harness's words")
}

func TestRegression_RealClaudeSubagentSession_SubagentToolCallsAreItsOwn(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	replayRealClaudeSession(t, f, runnerID)

	calls, err := f.activity.ToolCalls(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	names := map[string][]string{}
	for _, c := range calls {
		names[c.SubagentID] = append(names[c.SubagentID], c.Name)
	}
	assert.Equal(t, []string{"Agent", "Agent"}, names[""], "the main agent's own tool calls")
	for _, sub := range []string{"a3edc502687d15840", "a6fec6ffa95979d56"} {
		assert.Equal(t, []string{"Read", "Bash", "SubagentHandback"}, names[sub],
			"a subagent's tool calls are recorded under its scope, never the main agent's")
	}

	subs, err := f.activity.Subagents(f.ctx, chatID)
	require.NoError(t, err)
	assert.Len(t, subs, 2, "the subagents themselves are still tracked")
}

// running is the chat's ledger holding a turn open.
func running(t *testing.T, f testFixture, chatID string) bool {
	t.Helper()
	f.wait()
	st, err := f.ledger.State(f.ctx, chatID)
	require.NoError(t, err)
	_, ok := st.RunningTurn()
	return ok
}
