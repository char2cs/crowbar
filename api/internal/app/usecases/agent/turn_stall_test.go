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

// codexUsageLimitScreen is the REAL codex-cli 0.146.0 screen from the capture that
// found this defect: the usage-limit banner as it actually wrapped, across three
// rows at 100 columns, with the composer and its footer underneath.
//
// It is a capture and not an invention. A synthetic banner here would be the exact
// mistake this repo has made before — it would have been written unwrapped, and
// the wrapped continuation is the half of the sentence carrying the URL and the
// reset time, which is the whole reason a user wants to read it.
const codexUsageLimitScreen = "" +
	"■ You've hit your usage limit. Upgrade to Pro (https://chatgpt.com/explore/pro), visit\n" +
	"https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Aug 22nd, 2026\n" +
	"12:30 PM.\n" +
	"\n" +
	"› Implement {feature}\n" +
	"  ⏎ send   ⌃J newline   ⌃T transcript   ⌃C quit"

// codexUsageLimitSentence is that banner with the TERMINAL'S row breaks taken back
// out: what codex wrote, which is what the chat has to show.
const codexUsageLimitSentence = "" +
	"■ You've hit your usage limit. Upgrade to Pro (https://chatgpt.com/explore/pro), visit " +
	"https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Aug 22nd, 2026 " +
	"12:30 PM."

// TestRegression_StalledTurnIsClosedAndTheChatSaysWhy is defect 2, and the second
// half of defect 1.
//
// The measured failure: codex accepted a prompt, fired user_prompt, opened a turn,
// hit its usage limit, painted the reason on its own screen and stayed alive. The
// chat spun for 44 minutes — and the whole time, the explanation was on screen and
// the chat showed NOTHING.
//
// This drives the real thing end to end over the real event stores: a real spawn,
// a real user_prompt hook opening a real turn, the real descriptor matching the
// real captured screen, and the closer. Afterwards the chat is not working and its
// conversation carries codex's own sentence — including the wrapped continuation
// and the reset date, which is the part that tells the user when to come back.
func TestRegression_StalledTurnIsClosedAndTheChatSaysWhy(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")
	prompt(t, f, runnerID, "codex", "please do the thing")
	require.True(t, f.chat(t, chatID).Working, "the user's prompt must have opened a turn")

	// The notice comes from the SHIPPED descriptor matched against the CAPTURED
	// screen. Nothing here hand-builds the text the assertion then checks for.
	notice, ok := f.usecase.MatchTerminalNotice(f.ctx, "codex", codexUsageLimitScreen)
	require.True(t, ok, "the shipped codex descriptor must recognise its own usage-limit banner")

	agentusecase.CloseStalledTurn(f.usecase, f.ctx, termwait.Stall{
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

// TestUsecase_CloseStalledTurn_WritesNoNoticeWhenThereWasNoTurnToClose is the
// idempotence contract, and it matters because the detector decides from a
// projection that lags. A turn its own hook closed a moment ago can still read
// open to the sweep — and a notice written then would put "your provider gave up"
// underneath a turn that finished perfectly well.
func TestUsecase_CloseStalledTurn_WritesNoNoticeWhenThereWasNoTurnToClose(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sess-1")
	require.False(t, f.chat(t, chatID).Working, "no prompt was sent: there is no open turn")

	notice, ok := f.usecase.MatchTerminalNotice(f.ctx, "codex", codexUsageLimitScreen)
	require.True(t, ok)
	agentusecase.CloseStalledTurn(f.usecase, f.ctx, termwait.Stall{
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

// TestUsecase_MatchTerminalNotice_ResolvesTheShippedDescriptor drives the seam the
// detector reaches provider vocabulary through — home resolution, descriptor
// lookup, capability assertion, wrap-tolerant match, sentence capture — against
// the real embedded catalogue and the real captured screen.
func TestUsecase_MatchTerminalNotice_ResolvesTheShippedDescriptor(t *testing.T) {
	f := newFixture(t)

	notice, ok := f.usecase.MatchTerminalNotice(f.ctx, "codex", codexUsageLimitScreen)

	require.True(t, ok)
	assert.Equal(t, engineagents.TerminalNoticeUsageLimit, notice.Kind)
	assert.True(t, notice.EndsTurn, "the banner is painted because the attempt ended")
	assert.Equal(t, codexUsageLimitSentence, notice.Text)
	// The composer and the footer sit under a BLANK row, so the capture stops
	// where codex's sentence stops.
	assert.NotContains(t, notice.Text, "Implement {feature}")
	assert.NotContains(t, notice.Text, "transcript")
}

// TestUsecase_MatchTerminalNotice_ClaudeDeclaresNone is the degradation guarantee
// stated where it is relied on. claude's Stop hook is reliable and no notice of
// this shape has been measured from it, so claude declares none — and a provider
// declaring none never matches, even shown another provider's exact banner.
//
// This is what makes the whole mechanism safe to ship: a provider nobody has
// captured a screen from behaves byte-for-byte as it did before it existed.
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

// TestUsecase_MatchTerminalNotice_OrdinaryScreenIsNotANotice: the needle is a
// sentence codex only writes when it has given up, so an ordinary working screen
// matches nothing. Without this the mechanism would be an unconditional
// turn-closer with extra steps.
func TestUsecase_MatchTerminalNotice_OrdinaryScreenIsNotANotice(t *testing.T) {
	f := newFixture(t)

	_, ok := f.usecase.MatchTerminalNotice(f.ctx, "codex",
		"› Explain this codebase\n  ⏎ send   ⌃J newline")

	assert.False(t, ok)
}

// TestUsecase_OpenWork_ReportsAToolCallTheProviderNeverClosed is the gate that
// answers "is it working?" from hook evidence instead of from pixels — the one
// that covers codex, whose painting behaviour while working could not be measured
// (its account was rate-limited throughout).
//
// Driven through the real hook path, because that is the only thing that ever
// opens a tool call in production.
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

// TestUsecase_OpenWork_ReportsASubagentTheProviderNeverStopped is the same gate's
// other half: a subagent started and not stopped is work in flight too.
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

// TestUsecase_MatchTerminalNotice_SurvivesANarrowPane is the property the
// whole-screen match exists for, tested at the width the capture bound was sized
// against.
//
// At 24 columns codex's sentence wraps onto EIGHT rows — the cap exactly — and
// the needle itself is SPLIT across two of them, which a per-row substring search
// finds nowhere. The match still lands, and the captured text still reads back as
// the one sentence codex wrote.
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

// wrapAt breaks text onto rows of at most width COLUMNS on word boundaries — what
// a TUI does to a long sentence in a narrow pane.
//
// Columns are runes, not bytes. codex's banner opens with a three-byte bullet
// glyph, and measuring it in bytes would wrap the fixture a row earlier than the
// terminal being simulated ever would.
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

// orderLog records the sequence of the writes closeStalledTurn makes, so a test
// can assert on their ORDER rather than merely on their presence.
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

// orderedChats notes the command whose projection publishes Working=false.
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

// orderedActivity notes the ledger append that carries the explanation.
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

// TestRegression_StallNoticeIsDurableBeforeTheIdleEdgeIsPublished pins the ORDER,
// not just the presence of both effects.
//
// AbandonTurn's projection broadcasts Working=false, and the chat treats that edge
// as its cue to do ONE ledger read and then stop. A ledger append emits no frame
// of its own — so a notice appended AFTER that edge triggers nothing at all and
// sits invisible until an unrelated refresh happens to fire. The spinner would
// stop correctly and the chat would still never say why: defect 2 surviving the
// fix for defect 1.
//
// It is the same race handleTurn's turn_stop arm documents, observed live on
// 2026-08-16, and this is what stops it being reintroduced by a reordering that
// looks harmless.
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
	agentusecase.CloseStalledTurn(f.usecase, f.ctx, termwait.Stall{
		ChatID: chatID, WorkspaceID: "ws1", ProviderID: "codex",
		RunnerID: runnerID, SessionID: "sess-1", Notice: notice,
	})
	f.wait()

	assert.Equal(t, []string{"notice-appended", "abandon-turn"}, log.all(),
		"the explanation must be durable before anything publishes the idle edge")
}
