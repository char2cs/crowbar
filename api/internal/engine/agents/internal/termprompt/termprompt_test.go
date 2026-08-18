package termprompt_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/termprompt"
)

// claudeTrustScreen is the workspace-trust dialog as claude paints it, captured
// live from claude 2.1.207 and recorded in tests/integration/agent/barriers_test.go
// — the same capture the shipped descriptor's needles come from. Reproduced here so
// the matcher is tested against the real screen rather than against its own needles.
const claudeTrustScreen = `
╭───────────────────────────────────────────────╮
│ Do you trust the files in this folder?         │
│                                               │
│ /Users/somebody/code/project                  │
│                                               │
│ ❯ 1. Yes, I trust this folder                  │
│   2. No, exit                                  │
│                                               │
│ Enter to confirm · Esc to cancel               │
╰───────────────────────────────────────────────╯
`

// declaring mirrors claude.yaml: one KINDED needle that identifies the trust
// dialog, and one generic needle that only proves a modal is up.
func declaring() *spec.Descriptor {
	return &spec.Descriptor{
		ID: "claude",
		TerminalPrompts: []spec.TerminalPromptSpec{
			{Kind: spec.TerminalPromptTrust, Needle: "I trust this folder"},
			{Needle: "Enter to confirm"},
		},
	}
}

func TestMatch_IdentifiesTheTrustDialogFromItsRealScreen(t *testing.T) {
	got, ok := termprompt.Match(declaring(), claudeTrustScreen)

	assert.True(t, ok)
	assert.Equal(t, spec.TerminalPromptTrust, got.Kind)
	assert.Equal(t, "I trust this folder", got.Needle)
}

// TestMatch_SpecificWinsRegardlessOfOrder pins the rule that decides WHICH answer
// a screen matching both needles gets. Both orderings must name the trust dialog:
// which line a descriptor happens to list first is not a fact about the screen.
func TestMatch_SpecificWinsRegardlessOfOrder(t *testing.T) {
	reversed := &spec.Descriptor{
		ID: "claude",
		TerminalPrompts: []spec.TerminalPromptSpec{
			{Needle: "Enter to confirm"},
			{Kind: spec.TerminalPromptTrust, Needle: "I trust this folder"},
		},
	}

	got, ok := termprompt.Match(reversed, claudeTrustScreen)

	assert.True(t, ok)
	assert.Equal(t, spec.TerminalPromptTrust, got.Kind)
}

// TestMatch_UnidentifiedPromptReportsNoKind is the honest-fallback case: claude's
// modal footer with none of the trust text, which is what a login or migration
// prompt would look like. It must report the block WITHOUT naming it.
func TestMatch_UnidentifiedPromptReportsNoKind(t *testing.T) {
	got, ok := termprompt.Match(declaring(), "  Sign in to continue\n  Enter to confirm · Esc to cancel")

	assert.True(t, ok)
	assert.Empty(t, got.Kind)
	assert.Equal(t, "Enter to confirm", got.Needle)
}

// TestMatch_SurvivesWrappingAndPadding is why matching squeezes rather than
// substring-searching. A narrow pane genuinely breaks a needle across two screen
// rows, and a literal search finds neither half.
func TestMatch_SurvivesWrappingAndPadding(t *testing.T) {
	wrapped := "│   ❯ 1. Yes, I trust\n│   this folder      │"

	got, ok := termprompt.Match(declaring(), wrapped)

	assert.True(t, ok)
	assert.Equal(t, spec.TerminalPromptTrust, got.Kind)
}

func TestMatch_OrdinaryScreenMatchesNothing(t *testing.T) {
	_, ok := termprompt.Match(declaring(), "> Ready. Try \"fix the failing test\"\n  shift+tab to cycle")

	assert.False(t, ok)
}

// TestMatch_ProviderDeclaringNothingNeverMatches is the whole degradation story:
// a descriptor with no needles behaves exactly as it did before this existed, on
// every screen including one that would match another provider's needles.
func TestMatch_ProviderDeclaringNothingNeverMatches(t *testing.T) {
	silent := &spec.Descriptor{ID: "quiet"}

	_, ok := termprompt.Match(silent, claudeTrustScreen)

	assert.False(t, ok)
	assert.False(t, termprompt.Declared(silent))
}

func TestMatch_NilDescriptorAndEmptyScreenAreSafe(t *testing.T) {
	_, ok := termprompt.Match(nil, claudeTrustScreen)
	assert.False(t, ok)

	_, ok = termprompt.Match(declaring(), "")
	assert.False(t, ok)

	assert.False(t, termprompt.Declared(nil))
	assert.True(t, termprompt.Declared(declaring()))
}

// TestMatch_PunctuationOnlyScreenMatchesNothing guards the reduction's own edge:
// a screen of pure box drawing squeezes to the empty string, which must not be
// treated as containing anything.
func TestMatch_PunctuationOnlyScreenMatchesNothing(t *testing.T) {
	_, ok := termprompt.Match(declaring(), "╭──────╮\n│      │\n╰──────╯")

	assert.False(t, ok)
}

// TestMatch_PunctuationOnlyNeedleIsIgnored is the match-time half of the rule the
// descriptor validator enforces at load time. An all-punctuation needle reduces to
// "", which every screen contains — so it must be skipped rather than reporting
// every idle chat as blocked.
func TestMatch_PunctuationOnlyNeedleIsIgnored(t *testing.T) {
	_, ok := termprompt.Match(&spec.Descriptor{
		ID:              "sloppy",
		TerminalPrompts: []spec.TerminalPromptSpec{{Needle: "· ⏎"}},
	}, "an entirely ordinary screen")

	assert.False(t, ok)
}

// TestMatch_CodexPromptReportsGenericBlock pins the shipped codex declaration:
// neither line on its dialog mentions trust, so all it can truthfully say is that
// the CLI is blocked.
func TestMatch_CodexPromptReportsGenericBlock(t *testing.T) {
	codex := &spec.Descriptor{
		ID:              "codex",
		TerminalPrompts: []spec.TerminalPromptSpec{{Needle: "Press enter to continue"}},
	}
	screen := strings.Join([]string{
		"› 1. Yes, continue",
		"  2. No, exit",
		"  Press enter to continue",
	}, "\n")

	got, ok := termprompt.Match(codex, screen)

	assert.True(t, ok)
	assert.Empty(t, got.Kind)
}
