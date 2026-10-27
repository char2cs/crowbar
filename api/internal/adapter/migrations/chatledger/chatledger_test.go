package chatledger

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// No recording of a legacy home exists in the repository, so these chats are
// written by hand in the shape the old daemon's tables and journal held.
func at(h, m, s int) time.Time { return time.Date(2026, 9, 1, h, m, s, 0, time.UTC) }

func ptr(t time.Time) *time.Time { return &t }

// chatA: two conversations, the older with turns; three exchanges, a harness
// row, a notice; a tool and one nested under a subagent; an ended and an
// unended subagent; a compaction, a resolved permission, an uncertain prompt.
func chatA() LegacyChat {
	const p, run, chat = "claude", "run-1", "chat-a"
	turn := func(id, role string, seq, order int64, start time.Time, end *time.Time, text string) LegacyTurn {
		return LegacyTurn{
			ID: id, ChatID: chat, Role: role, ProviderID: p, RunnerID: run, SessionID: "S-old",
			Text: text, Seq: seq, DisplayOrder: order, StartedAt: start, EndedAt: end,
		}
	}
	turns := []LegacyTurn{
		turn("req-1", roleUser, 1, 1, at(10, 1, 0), nil, "first prompt"),
		turn("msg-1", roleAssistant, 3, 1, at(10, 2, 0), ptr(at(10, 2, 30)), "first answer"),
		turn("u-2", roleUser, 4, 2, at(10, 5, 0), nil, "typed in the CLI"),
		turn("msg-2", roleAssistant, 7, 2, at(10, 6, 0), ptr(at(10, 6, 30)), "second answer"),
		turn("u-3", roleUser, 9, 3, at(10, 12, 0), nil, "third prompt"),
		turn("msg-3", roleAssistant, 11, 3, at(10, 14, 0), ptr(at(10, 14, 30)), "third answer"),
		turn("h-1", roleHarness, 12, 4, at(10, 15, 0), nil, "harness note"),
		turn("n-1", roleNotice, 13, 4, at(10, 16, 0), nil, "usage limit reached"),
	}
	turns[1].Effort, turns[1].Plan = "high", `[{"text":"look","status":"completed"}]`
	return LegacyChat{
		ChatID: chat,
		Conversations: []LegacyConversation{
			{ChatID: chat, ProviderID: p, SessionID: "S-old", FirstSeenAt: at(10, 0, 0), LastActiveAt: at(10, 30, 0)},
			{ChatID: chat, ProviderID: p, SessionID: "S-new", FirstSeenAt: at(10, 40, 0), LastActiveAt: at(10, 45, 0)},
		},
		Turns: turns,
		Tools: []LegacyTool{
			{
				ID: "t-1", TurnID: "open-chat-a-run-1", ChatID: chat, Name: "Read", Kind: "read", Seq: 2,
				Locations: `[{"path":"/w/a.go","line":3}]`, RequestRef: "req/t-1", ResultRef: "res/t-1",
				Status: "ok", DurationMS: 40, StartedAt: at(10, 1, 30), EndedAt: ptr(at(10, 1, 31)),
			},
			{
				ID: "t-2", ChatID: chat, Name: "Bash", Kind: "execute", SubagentID: "sa-1", Seq: 6,
				RequestRef: "req/t-2", ResultRef: "res/t-2", Status: "error", Error: "exit 1",
				StartedAt: at(10, 5, 40), EndedAt: ptr(at(10, 5, 41)),
			},
		},
		Subagents: []LegacySubagent{
			{
				ID: "sa-1", TurnID: "msg-2", ChatID: chat, AgentType: "explore", Seq: 5,
				Messages: `[{"text":"looking"},{"text":"found it"}]`, StartedAt: at(10, 5, 30), EndedAt: ptr(at(10, 5, 50)),
			},
			{ID: "sa-2", TurnID: "msg-3", ChatID: chat, AgentType: "general", Seq: 10, StartedAt: at(10, 13, 0)},
		},
		Interruptions: []LegacyInterruption{
			{ID: "i-2", TurnID: "msg-1", ChatID: chat, Kind: kindPermission, Seq: 2, DisplayOrder: 1, At: at(10, 1, 40)},
			{
				ID: "i-1", TurnID: "msg-2", ChatID: chat, Kind: kindCompaction, Seq: 8, DisplayOrder: 2,
				At: at(10, 7, 0), ResolvedAt: ptr(at(10, 7, 10)),
			},
		},
		Choices: []LegacyChoice{
			{
				ID: "ch-1", TurnID: "msg-1", ChatID: chat, Kind: "tool_permission", ToolID: "t-1", ToolName: "Read",
				Title: "Allow Read?", Options: `[{"id":"allow","kind":"allow","label":"Allow"}]`, Seq: 2,
				At: at(10, 1, 40), ResolvedAt: ptr(at(10, 1, 45)), Resolution: "answered", AnsweredOptionIDs: `["allow"]`,
			},
		},
		Prompts: []LegacyPrompt{
			{RequestID: "req-1", Text: "first prompt", State: "accepted", ProviderID: p, CreatedAt: at(10, 0, 50), UpdatedAt: at(10, 1, 0)},
			{RequestID: "req-9", Text: "lost prompt", State: "uncertain", ProviderID: p, CreatedAt: at(10, 20, 0), UpdatedAt: at(10, 20, 5)},
		},
	}
}

// chatB: another provider; turns that name no session; an inferred
// interruption on a turn with no reply; its only session owned by another chat.
func chatB() LegacyChat {
	const p, chat = "codex", "chat-b"
	return LegacyChat{
		ChatID: chat,
		Conversations: []LegacyConversation{
			{ChatID: chat, ProviderID: p, SessionID: "S-b", FirstSeenAt: at(9, 0, 0), LastActiveAt: at(9, 30, 0)},
		},
		Turns: []LegacyTurn{
			{ID: "u-b1", ChatID: chat, Role: roleUser, ProviderID: p, RunnerID: "run-b", Text: "hi", Seq: 1, DisplayOrder: 1, StartedAt: at(9, 1, 0)},
			{
				ID: "msg-b1", ChatID: chat, Role: roleAssistant, ProviderID: p, RunnerID: "run-b", Text: "hello", Seq: 2, DisplayOrder: 1,
				StartedAt: at(9, 2, 0), EndedAt: ptr(at(9, 2, 5)),
			},
			{ID: "u-b2", ChatID: chat, Role: roleUser, ProviderID: p, RunnerID: "run-b", Text: "and then", Seq: 3, DisplayOrder: 2, StartedAt: at(9, 10, 0)},
		},
		Interruptions: []LegacyInterruption{
			{ID: "i-b1", TurnID: "open-chat-b-run-b", ChatID: chat, Kind: kindInferred, Seq: 4, DisplayOrder: 2, At: at(9, 11, 0)},
		},
	}
}

func nobody(string, string) (string, bool) { return "", false }

func convert(t *testing.T, in LegacyChat, owner Owner) Result {
	t.Helper()
	res, err := ConvertChat(in, owner)
	require.NoError(t, err)
	return res
}

func fold(res Result) ledger.State { return ledger.Fold(res.Rows[0].ChatID, res.Rows) }

func rowsOf(res Result, kind ledger.Kind) []ledger.Row {
	var out []ledger.Row
	for _, r := range res.Rows {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}

func body[B any](t *testing.T, r ledger.Row) B {
	t.Helper()
	b, err := ledger.DecodeBody[B](r)
	require.NoError(t, err)
	return b
}

func bodies[B any](t *testing.T, res Result, kind ledger.Kind) []B {
	t.Helper()
	var out []B
	for _, r := range rowsOf(res, kind) {
		out = append(out, body[B](t, r))
	}
	return out
}

func TestConvertChat_IsDeterministic(t *testing.T) {
	for _, in := range []LegacyChat{chatA(), chatB()} {
		first, second := convert(t, in, nobody), convert(t, in, nobody)

		assert.True(t, reflect.DeepEqual(first, second), in.ChatID)
	}
	assert.Equal(t, turnID("chat-a", "req-1"), rowsOf(convert(t, chatA(), nobody), ledger.KindTurnStarted)[0].TurnID,
		"a turn's id derives from its chat and legacy id alone")
}

func TestConvertChat_FoldsToARestingChat(t *testing.T) {
	for _, in := range []LegacyChat{chatA(), chatB()} {
		res := convert(t, in, nobody)
		st := ledger.State{ChatID: in.ChatID}
		for i, r := range res.Rows {
			assert.Equal(t, int64(i+1), r.Seq)
			assert.Equal(t, in.ChatID, r.ChatID)
			require.NoError(t, ledger.Check(st, r))
			if i > 0 {
				assert.False(t, r.At.Before(res.Rows[i-1].At), "rows are in time order")
			}
			st.Apply(r)
		}

		assert.False(t, st.HasOpenWork(), in.ChatID)
		assert.NoError(t, AtRest(st, nil))
	}
}

func TestConvertChat_TurnsBecomePromptTextAndAnEnd(t *testing.T) {
	res := convert(t, chatA(), nobody)

	messages := bodies[ledger.UserMessage](t, res, ledger.KindUserMessage)
	require.GreaterOrEqual(t, len(messages), 3)
	assert.Equal(t, ledger.UserMessage{RequestID: "req-1", Text: "first prompt", Origin: ledger.OriginCrowbar}, messages[0],
		"a prompt the journal holds is Crowbar's")
	assert.Equal(t, ledger.UserMessage{Text: "typed in the CLI", Origin: ledger.OriginProvider}, messages[1])
	assert.Equal(t, "req-1", bodies[ledger.TurnStarted](t, res, ledger.KindTurnStarted)[0].RequestID, "its turn delivers it")
	texts := bodies[ledger.AssistantText](t, res, ledger.KindAssistantText)
	assert.Equal(t, ledger.AssistantText{MessageID: "msg-1", Index: ledger.IndexComplete, Final: true, Text: "first answer"}, texts[0])
	ends := bodies[ledger.TurnEnded](t, res, ledger.KindTurnEnded)
	assert.Equal(t, ledger.TurnEnded{Status: "done", RunnerID: "run-1", Effort: "high"}, ends[0])
	assert.Equal(t, []ledger.PlanItem{{Text: "look", Status: "completed"}}, fold(res).Plan(), "the plan is the turn's own")
}

// A reply the old daemon stopped short of recording ends interrupted: a resting
// chat has no running turn.
func TestConvertChat_AReplyWithNoEndIsInterrupted(t *testing.T) {
	in := chatA()
	in.Turns[3].EndedAt = nil

	res := convert(t, in, nobody)

	ends := bodies[ledger.TurnEnded](t, res, ledger.KindTurnEnded)
	assert.Equal(t, ledger.TurnEnded{Status: "interrupted", Reason: ledger.ReasonDaemonRestart, RunnerID: "run-1"}, ends[1])
	assert.NoError(t, AtRest(fold(res), nil))
}
