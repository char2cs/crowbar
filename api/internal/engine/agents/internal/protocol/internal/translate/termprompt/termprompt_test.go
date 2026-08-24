package termprompt_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/termprompt"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

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

func TestMatch_UnidentifiedPromptReportsNoKind(t *testing.T) {
	got, ok := termprompt.Match(declaring(), "  Sign in to continue\n  Enter to confirm · Esc to cancel")

	assert.True(t, ok)
	assert.Empty(t, got.Kind)
	assert.Equal(t, "Enter to confirm", got.Needle)
}

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

func TestMatch_PunctuationOnlyScreenMatchesNothing(t *testing.T) {
	_, ok := termprompt.Match(declaring(), "╭──────╮\n│      │\n╰──────╯")

	assert.False(t, ok)
}

func TestMatch_PunctuationOnlyNeedleIsIgnored(t *testing.T) {
	_, ok := termprompt.Match(&spec.Descriptor{
		ID:              "sloppy",
		TerminalPrompts: []spec.TerminalPromptSpec{{Needle: "· ⏎"}},
	}, "an entirely ordinary screen")

	assert.False(t, ok)
}

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
