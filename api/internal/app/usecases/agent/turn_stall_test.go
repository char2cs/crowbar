package agent_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/agent"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

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

	agentusecase.CloseStalledTurn(f.usecase.TurnUsecase, f.ctx, termwait.Stall{
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
	agentusecase.CloseStalledTurn(f.usecase.TurnUsecase, f.ctx, termwait.Stall{
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
) (domain.AgentChat, error) {
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
	agentusecase.CloseStalledTurn(f.usecase.TurnUsecase, f.ctx, termwait.Stall{
		ChatID: chatID, WorkspaceID: "ws1", ProviderID: "codex",
		RunnerID: runnerID, SessionID: "sess-1", Notice: notice,
	})
	f.wait()

	assert.Equal(t, []string{"notice-appended", "abandon-turn"}, log.all(),
		"the explanation must be durable before anything publishes the idle edge")
}
