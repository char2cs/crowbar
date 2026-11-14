package agents_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

// The screens an agent's terminal paints while it blocks on a prompt only it can answer.

// A claude never set up on this machine paints its onboarding before any
// hook: both screens read as blocking, never as a healthy empty pane.
func TestMatchTerminalPrompt_ClaudeFirstRunOnboardingBlocks(t *testing.T) {
	for _, screen := range []string{
		"Welcome to Claude Code v2.1.281\n Let's get started.\n Choose the text style that looks best with your terminal\n ❯ 2. Dark mode ✔",
		"Claude Code can be used with your Claude subscription\n Select login method:\n ❯ Claude account with subscription",
	} {
		_, ok := get(t, "claude").MatchTerminalPrompt(screen)
		assert.True(t, ok, "not recognised as blocking: %q", screen)
	}
}

// A command's dialog (2.1.319) blocks: by its footer, or by its tab row where a small grid cuts it.
func TestMatchTerminalPrompt_ClaudeCommandDialogBlocks(t *testing.T) {
	for _, screen := range []string{" For more help: …\n Esc to cancel", "  Help   General   Commands   Custom commands\n"} {
		prompt, ok := get(t, "claude").MatchTerminalPrompt(screen)
		assert.True(t, ok && prompt.Kind == "", screen)
	}
}

// The trust dialog as the real CLI paints it (2.1.319, through our emulator) preselects
// "No, exit": accepting must move the cursor onto the trust option before confirming.
func TestMatchTerminalPrompt_ClaudeAimsItsTrustAnswerAtTheTrustOption(t *testing.T) {
	screen, err := os.ReadFile("testdata/claude-trust-2.1.319.screen")
	require.NoError(t, err)

	prompt, ok := get(t, "claude").MatchTerminalPrompt(string(screen))

	require.True(t, ok)
	assert.Equal(t, agents.TerminalPromptTrust, prompt.Kind)
	require.Len(t, prompt.Answers, 1)
	assert.Equal(t, "\x1b[B\r", prompt.Answers[0].Keys)
}

func TestMatchTerminalPrompt_CodexReportsAGenericBlock(t *testing.T) {
	screen := "› 1. Yes, continue\n  2. No, exit\n  Press enter to continue"

	prompt, ok := get(t, "codex").MatchTerminalPrompt(screen)

	require.True(t, ok)
	assert.Empty(t, prompt.Kind, "codex declares no kinded needle; naming one would be a guess")
}

func TestMatchTerminalPrompt_AnOrdinaryScreenIsNotABlock(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		_, ok := get(t, id).MatchTerminalPrompt("> Ready.\n  shift+tab to cycle · ? for shortcuts")
		assert.False(t, ok, id)
	}
}

func TestCapabilities_TerminalPromptsIsDeclaredByBothShippedAgents(t *testing.T) {
	assert.True(t, get(t, "claude").Capabilities().TerminalPrompts)
	assert.True(t, get(t, "codex").Capabilities().TerminalPrompts)
}

func TestMatchTerminalPrompt_ProviderDeclaringNoneNeverMatches(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "descriptors", "silent.yaml"), []byte(
		"id: silent\nspawn:\n  cmd: silent-cli\n  interactive_required: true\n"+
			"hooks:\n  format: json\n  events:\n"+
			"    session_start: { session_id: session_id }\n"+
			"    turn_stop: { message: last }\n",
	), 0o600))

	a, err := agents.New().Get(context.Background(), home, "silent")
	require.NoError(t, err)

	_, ok := a.MatchTerminalPrompt("❯ 1. Yes, I trust this folder\nEnter to confirm")
	assert.False(t, ok)
	assert.False(t, a.Capabilities().TerminalPrompts)
}

// composerRecording is a screen recorded from the real CLI through our emulator
// plus the cursor it left, as the terminal read reports them.
func composerRecording(t *testing.T, state string) (screen string, row, col int) {
	t.Helper()
	text, err := os.ReadFile("testdata/claude-composer-" + state + "-2.1.325.screen")
	require.NoError(t, err)
	cursor, err := os.ReadFile("testdata/claude-composer-" + state + "-2.1.325.cursor")
	require.NoError(t, err)
	var paste bool
	_, err = fmt.Sscanf(string(cursor), "%d %d %t", &row, &col, &paste)
	require.NoError(t, err)
	return string(text), row, col
}

// typistWithPrefix is the typist descriptor with its composer row prefix swapped.
func typistWithPrefix(t *testing.T, prefix string) agents.Agent {
	t.Helper()
	home := t.TempDir()
	body := strings.Replace(typedPromptDescriptor, `row_prefix: "❯ "`, "row_prefix: "+strconv.Quote(prefix), 1)
	require.NotEqual(t, typedPromptDescriptor, body)
	writeDescriptor(t, home, "typist", body)
	a, err := agents.New().Get(context.Background(), home, "typist")
	require.NoError(t, err)
	return a
}

func TestComposerEmpty_ClaudeReadsEachRecordedScreen(t *testing.T) {
	cases := []struct {
		state string
		empty bool
	}{
		{"idle-fresh-80x24", true},
		{"idle-fresh-120x40", true},
		{"idle-ghost-80x24", true},
		{"idle-ghost-120x40", true},
		// The composer reads empty mid-turn too: the ledger is what guards it.
		{"midturn-80x24", true},
		{"draft-80x24", false},
		{"draft-120x40", false},
		{"help-dialog-80x24", false},
		{"paste-bang-80x24", false},
		{"paste-escaped-bang-80x24", false},
		{"paste-multiline-80x24", false},
		{"slash-typed-80x24", false},
	}
	for _, tc := range cases {
		t.Run(tc.state, func(t *testing.T) {
			screen, row, col := composerRecording(t, tc.state)

			assert.Equal(t, tc.empty, get(t, "claude").ComposerEmpty(screen, row, col))
		})
	}
}

func TestComposerEmpty_APlainSpacePrefixNeverMatchesTheNoBreakSpaceRecording(t *testing.T) {
	screen, row, col := composerRecording(t, "idle-fresh-80x24")

	assert.False(t, typistWithPrefix(t, "❯ ").ComposerEmpty(screen, row, col))
	assert.True(t, typistWithPrefix(t, "❯ ").ComposerEmpty(screen, row, col))
}

func TestComposerEmpty_JudgesTheCursorRowAgainstThePrefixDisplayWidth(t *testing.T) {
	a := typistWithPrefix(t, "あ> ")
	screen := "banner\nあ> \nfooter"

	assert.True(t, a.ComposerEmpty(screen, 1, 4), "a wide glyph takes two cells: 2 + '>' + ' '")
	assert.False(t, a.ComposerEmpty(screen, 1, 3), "the rune count is not the width")
	assert.False(t, a.ComposerEmpty(screen, 1, 5))
	assert.False(t, a.ComposerEmpty(screen, 0, 4), "another row does not start with the prefix")
	assert.False(t, a.ComposerEmpty(screen, 3, 4), "past the last row")
	assert.False(t, a.ComposerEmpty(screen, -1, 4), "before the first row")
}

func TestComposerEmpty_ProviderDeclaringNoTypeBlockIsNeverEmpty(t *testing.T) {
	screen, row, col := composerRecording(t, "idle-fresh-80x24")

	assert.False(t, get(t, "codex").ComposerEmpty(screen, row, col))
}
