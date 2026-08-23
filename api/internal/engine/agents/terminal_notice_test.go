package agents_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

const codexUsageLimitScreen = "" +
	"■ You've hit your usage limit. Upgrade to Pro (https://chatgpt.com/explore/pro), visit\n" +
	"https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Aug 22nd, 2026\n" +
	"12:30 PM.\n"

func TestAgent_MatchTerminalNotice_ReadsTheShippedCodexDescriptor(t *testing.T) {
	a, err := engineagents.New().Get(context.Background(), t.TempDir(), "codex")
	require.NoError(t, err)

	matcher, ok := a.(engineagents.NoticeMatcher)
	require.True(t, ok, "the concrete agent must satisfy the capability interface")

	notice, ok := matcher.MatchTerminalNotice(codexUsageLimitScreen)

	require.True(t, ok)
	assert.Equal(t, engineagents.TerminalNoticeUsageLimit, notice.Kind)
	assert.True(t, notice.EndsTurn)
	assert.Contains(t, notice.Text, "You've hit your usage limit")
	assert.Contains(t, notice.Text, "Aug 22nd, 2026 12:30 PM")
}

func TestAgent_MatchTerminalNotice_ClaudeDeclaresNone(t *testing.T) {
	a, err := engineagents.New().Get(context.Background(), t.TempDir(), "claude")
	require.NoError(t, err)

	matcher, ok := a.(engineagents.NoticeMatcher)
	require.True(t, ok)

	_, matched := matcher.MatchTerminalNotice(codexUsageLimitScreen)

	assert.False(t, matched)
}
