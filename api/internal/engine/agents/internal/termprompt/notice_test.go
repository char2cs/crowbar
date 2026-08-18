package termprompt_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/termprompt"
)

// codexUsageLimitScreen is the REAL codex-cli 0.146.0 screen from the capture that
// found the wedge: the usage-limit banner wrapped across three rows at 100
// columns, then a blank row, then the composer and its footer.
//
// It is a capture, never an invention. A hand-written banner would have been
// written on one line, and the wrapped continuation is exactly what the capture
// rule has to reassemble — it holds the URL and the reset time.
const codexUsageLimitScreen = `
■ You've hit your usage limit. Upgrade to Pro (https://chatgpt.com/explore/pro), visit
https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Aug 22nd, 2026
12:30 PM.

› Implement {feature}
  ⏎ send   ⌃J newline   ⌃T transcript   ⌃C quit
`

// codexUsageLimitSentence is that banner with the TERMINAL'S row breaks taken back
// out — what codex wrote, before the pane width happened to it.
const codexUsageLimitSentence = "■ You've hit your usage limit. Upgrade to Pro " +
	"(https://chatgpt.com/explore/pro), visit https://chatgpt.com/codex/settings/usage " +
	"to purchase more credits or try again at Aug 22nd, 2026 12:30 PM."

// noticing mirrors codex.yaml: the one notice measured off a real CLI.
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

// TestMatchNotice_CapturesTheWrappedContinuation is the whole point of the capture
// rule. The needle is on the first row; the plan URL, the credits URL and the
// reset time are on the two rows the terminal wrapped them onto, and a rule that
// quoted only the matched row would drop every fact the user needs.
func TestMatchNotice_CapturesTheWrappedContinuation(t *testing.T) {
	got, _ := termprompt.MatchNotice(noticing(), codexUsageLimitScreen)

	assert.Contains(t, got.Text, "https://chatgpt.com/codex/settings/usage")
	assert.Contains(t, got.Text, "Aug 22nd, 2026 12:30 PM")
}

// TestMatchNotice_StopsAtTheBlankRow: a blank row is where a TUI ends one block of
// output and starts the next, so the composer and the key hints underneath codex's
// banner are not part of what codex said.
func TestMatchNotice_StopsAtTheBlankRow(t *testing.T) {
	got, _ := termprompt.MatchNotice(noticing(), codexUsageLimitScreen)

	assert.NotContains(t, got.Text, "Implement {feature}")
	assert.NotContains(t, got.Text, "transcript")
}

// TestMatchNotice_BoundsTheCapture is the pathological screen: a CLI that paints
// its banner and then a wall of non-blank rows with no separator anywhere. Eight
// rows of a provider's own words is a message; eighty would be a screen dump
// pasted into somebody's conversation.
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

// TestMatchNotice_CaptureRunsToTheBottomOfTheScreen: the last row of a screen has
// no blank row after it, so the loop has to end on the row count as well as on a
// separator. Without that it would read past the end of the grid.
func TestMatchNotice_CaptureRunsToTheBottomOfTheScreen(t *testing.T) {
	screen := "You've hit your usage limit.\nTry again later."

	got, ok := termprompt.MatchNotice(noticing(), screen)

	require.True(t, ok)
	assert.Equal(t, "You've hit your usage limit. Try again later.", got.Text)
}

// TestMatchNotice_SurvivesANarrowPane is the reason matching is done across the
// FLATTENED screen rather than row by row. At 24 columns the needle itself is
// split over two rows, so a per-row substring search finds nothing at all — and
// the sentence still comes back whole.
func TestMatchNotice_SurvivesANarrowPane(t *testing.T) {
	narrow := "■ You've hit your\nusage limit. Upgrade to\nPro, visit later."

	got, ok := termprompt.MatchNotice(noticing(), narrow)

	require.True(t, ok)
	assert.Equal(t, "■ You've hit your usage limit. Upgrade to Pro, visit later.", got.Text)
}

// TestMatchNotice_ProviderDeclaringNothingNeverMatches is the degradation
// guarantee this whole mechanism's safety rests on, and it is claude's entire
// relationship with it: declare nothing, match nothing, behave exactly as before.
func TestMatchNotice_ProviderDeclaringNothingNeverMatches(t *testing.T) {
	_, ok := termprompt.MatchNotice(&spec.Descriptor{ID: "claude"}, codexUsageLimitScreen)

	assert.False(t, ok)
}

// TestMatchNotice_EndsTurnIsCarriedNotAssumed: a declared notice that does not
// claim to end a turn must report exactly that, because ends_turn is the entire
// evidential claim a caller acts on. Silently upgrading it here would give every
// informational message the power to close a turn.
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

// TestMatchNotice_FirstDeclaredMatchWins. Unlike prompts there is no
// generic-versus-specific ranking to apply — every notice carries a kind — so
// declaration order is the descriptor author's own priority and is honoured.
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

// TestMatchNotice_NilDescriptorAndEmptyScreenAreSafe: this runs on a cadence over
// whatever the census hands it, so every degenerate input has to answer "no"
// rather than panic.
func TestMatchNotice_NilDescriptorAndEmptyScreenAreSafe(t *testing.T) {
	_, ok := termprompt.MatchNotice(nil, codexUsageLimitScreen)
	assert.False(t, ok)

	_, ok = termprompt.MatchNotice(noticing(), "")
	assert.False(t, ok)

	// A screen of pure punctuation reduces to nothing, and nothing must not match
	// everything.
	_, ok = termprompt.MatchNotice(noticing(), "··· ─── ⏎")
	assert.False(t, ok)
}

// TestMatchNotice_PunctuationOnlyNeedleIsIgnored: such a needle reduces to the
// empty string, which every screen contains — so it would close every working
// chat's turn. The descriptor validator refuses one, and the matcher refuses it
// again here, because the two failures are far too expensive to guard once.
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
