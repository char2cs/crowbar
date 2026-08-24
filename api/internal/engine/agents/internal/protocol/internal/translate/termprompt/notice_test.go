package termprompt_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/termprompt"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

const codexUsageLimitScreen = `
■ You've hit your usage limit. Upgrade to Pro (https://chatgpt.com/explore/pro), visit
https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Aug 22nd, 2026
12:30 PM.

› Implement {feature}
  ⏎ send   ⌃J newline   ⌃T transcript   ⌃C quit
`

const codexUsageLimitSentence = "■ You've hit your usage limit. Upgrade to Pro " +
	"(https://chatgpt.com/explore/pro), visit https://chatgpt.com/codex/settings/usage " +
	"to purchase more credits or try again at Aug 22nd, 2026 12:30 PM."

func noticing() *spec.Descriptor {
	return &spec.Descriptor{
		ID: "codex",
		TerminalNotices: []spec.TerminalNoticeSpec{
			{Kind: spec.TerminalNoticeUsageLimit, Needle: "You've hit your usage limit", EndsTurn: true},
		},
	}
}

func TestMatchNotice_QuotesTheProvidersOwnSentence(t *testing.T) {
	got, ok := termprompt.MatchNotice(noticing(), codexUsageLimitScreen)

	require.True(t, ok)
	assert.Equal(t, spec.TerminalNoticeUsageLimit, got.Kind)
	assert.True(t, got.EndsTurn)
	assert.Equal(t, codexUsageLimitSentence, got.Text)
}

func TestMatchNotice_CapturesTheWrappedContinuation(t *testing.T) {
	got, _ := termprompt.MatchNotice(noticing(), codexUsageLimitScreen)

	assert.Contains(t, got.Text, "https://chatgpt.com/codex/settings/usage")
	assert.Contains(t, got.Text, "Aug 22nd, 2026 12:30 PM")
}

func TestMatchNotice_StopsAtTheBlankRow(t *testing.T) {
	got, _ := termprompt.MatchNotice(noticing(), codexUsageLimitScreen)

	assert.NotContains(t, got.Text, "Implement {feature}")
	assert.NotContains(t, got.Text, "transcript")
}

func TestMatchNotice_BoundsTheCapture(t *testing.T) {
	rows := []string{"You've hit your usage limit."}
	for i := 0; i < 40; i++ {
		rows = append(rows, "filler row that never goes blank")
	}

	got, ok := termprompt.MatchNotice(noticing(), strings.Join(rows, "\n"))

	require.True(t, ok)
	assert.Equal(t, 8, strings.Count(got.Text, "filler row that never goes blank")+1,
		"the matched row plus seven continuations, and no more")
}

func TestMatchNotice_CaptureRunsToTheBottomOfTheScreen(t *testing.T) {
	screen := "You've hit your usage limit.\nTry again later."

	got, ok := termprompt.MatchNotice(noticing(), screen)

	require.True(t, ok)
	assert.Equal(t, "You've hit your usage limit. Try again later.", got.Text)
}

func TestMatchNotice_SurvivesANarrowPane(t *testing.T) {
	narrow := "■ You've hit your\nusage limit. Upgrade to\nPro, visit later."

	got, ok := termprompt.MatchNotice(noticing(), narrow)

	require.True(t, ok)
	assert.Equal(t, "■ You've hit your usage limit. Upgrade to Pro, visit later.", got.Text)
}

func TestMatchNotice_ProviderDeclaringNothingNeverMatches(t *testing.T) {
	_, ok := termprompt.MatchNotice(&spec.Descriptor{ID: "claude"}, codexUsageLimitScreen)

	assert.False(t, ok)
}

func TestMatchNotice_EndsTurnIsCarriedNotAssumed(t *testing.T) {
	informational := &spec.Descriptor{
		ID: "codex",
		TerminalNotices: []spec.TerminalNoticeSpec{
			{Kind: spec.TerminalNoticeUsageLimit, Needle: "You've hit your usage limit"},
		},
	}

	got, ok := termprompt.MatchNotice(informational, codexUsageLimitScreen)

	require.True(t, ok)
	assert.False(t, got.EndsTurn)
}

func TestMatchNotice_FirstDeclaredMatchWins(t *testing.T) {
	two := &spec.Descriptor{
		ID: "codex",
		TerminalNotices: []spec.TerminalNoticeSpec{
			{Kind: spec.TerminalNoticeUsageLimit, Needle: "hit your usage limit", EndsTurn: true},
			{Kind: spec.TerminalNoticeUsageLimit, Needle: "Upgrade to Pro"},
		},
	}

	got, ok := termprompt.MatchNotice(two, codexUsageLimitScreen)

	require.True(t, ok)
	assert.Equal(t, "hit your usage limit", got.Needle)
}

func TestMatchNotice_OrdinaryScreenMatchesNothing(t *testing.T) {
	_, ok := termprompt.MatchNotice(noticing(), "› Explain this codebase\n  ⏎ send   ⌃J newline")

	assert.False(t, ok)
}

func TestMatchNotice_NilDescriptorAndEmptyScreenAreSafe(t *testing.T) {
	_, ok := termprompt.MatchNotice(nil, codexUsageLimitScreen)
	assert.False(t, ok)

	_, ok = termprompt.MatchNotice(noticing(), "")
	assert.False(t, ok)

	_, ok = termprompt.MatchNotice(noticing(), "··· ─── ⏎")
	assert.False(t, ok)
}

func TestMatchNotice_PunctuationOnlyNeedleIsIgnored(t *testing.T) {
	junk := &spec.Descriptor{
		ID: "codex",
		TerminalNotices: []spec.TerminalNoticeSpec{
			{Kind: spec.TerminalNoticeUsageLimit, Needle: "··· ⏎", EndsTurn: true},
		},
	}

	_, ok := termprompt.MatchNotice(junk, codexUsageLimitScreen)

	assert.False(t, ok)
}
