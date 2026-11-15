package agents_test

import (
	"context"
	"os"
	"path/filepath"
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
