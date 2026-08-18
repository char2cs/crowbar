package agents_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// codexUsageLimitScreen is the REAL codex-cli 0.146.0 screen from the capture that
// found the wedge, wrapped exactly as the terminal wrapped it at 100 columns.
const codexUsageLimitScreen = "" +
	"■ You've hit your usage limit. Upgrade to Pro (https://chatgpt.com/explore/pro), visit\n" +
	"https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Aug 22nd, 2026\n" +
	"12:30 PM.\n"

// TestAgent_MatchTerminalNotice_ReadsTheShippedCodexDescriptor drives the public
// seam end to end: resolve the embedded codex descriptor, assert it satisfies the
// capability interface, and match the captured screen through it.
//
// The capability assertion is the load-bearing half. A caller reaches this through
// a TYPE ASSERTION, which cannot fail at build time — so an agent that stopped
// implementing it would not break a build, it would silently stop closing wedged
// turns in production and nowhere else.
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

// TestAgent_MatchTerminalNotice_ClaudeDeclaresNone is the degradation guarantee at
// the engine's own boundary: claude implements the capability and answers false
// for everything, because it declares no notices at all.
func TestAgent_MatchTerminalNotice_ClaudeDeclaresNone(t *testing.T) {
	a, err := engineagents.New().Get(context.Background(), t.TempDir(), "claude")
	require.NoError(t, err)

	matcher, ok := a.(engineagents.NoticeMatcher)
	require.True(t, ok)

	_, matched := matcher.MatchTerminalNotice(codexUsageLimitScreen)

	assert.False(t, matched)
}
