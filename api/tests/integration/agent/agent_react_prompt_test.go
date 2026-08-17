//go:build integration

package agent_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/ledger"
	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestAgent_ReactPromptRestartsInteractiveTUI proves the production Chat-view
// transport against both real provider CLIs. A prompt is delivered only as the
// final positional argument of a replacement interactive TUI: this test never
// writes a byte into either replacement PTY. Provider hooks must record the
// exact user text and a complete assistant response, and the TUI must remain
// alive after each turn so Terminal view can attach to the same process.
func TestAgent_ReactPromptRestartsInteractiveTUI(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		provider := provider
		t.Run(provider, func(t *testing.T) {
			requireCLI(t, provider)
			h := newHarness(t)
			ctx := context.Background()

			repoPath := kit.InitRepo(t)
			_, _, wsID := h.importRepoAndWorkspace(t, "react-prompt-"+provider, repoPath)

			chatID, idleRunnerID, idleTermID, idleTap := spawnReady(t, h, wsID, provider)
			if provider == "claude" {
				// Claude announces an otherwise-empty native session at boot. Let
				// that hook settle before replacing the idle TUI so a late hook from
				// the outgoing runner cannot be mistaken for replacement activity.
				_, _ = awaitSessionBound(t, h, idleRunnerID, idleTermID, idleTap)
			}

			firstWant := fmt.Sprintf("CROWBAR-%s-FIRST", provider)
			firstText := "--crowbar-leading-dash=Reply with only the exact text " + firstWant
			first, err := h.app.Usecases.Agent.SubmitPrompt(ctx, chatID, firstText, uuid.NewString())
			require.NoError(t, err)
			require.NotEqual(t, idleRunnerID, first.RunnerID)
			require.NotEqual(t, idleTermID, first.TerminalSessionID)
			require.False(t, h.eng.Terminal.SessionLive(ctx, idleTermID),
				"the outgoing idle TUI must be stopped before the replacement becomes authoritative")

			firstTap := kit.AttachPTY(t, h.eng.Terminal, first.TerminalSessionID)
			t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), first.TerminalSessionID) })
			firstUser, firstAssistant := awaitPositionalPromptTurn(t, h, chatID, provider, 0, firstText)
			require.Equal(t, firstText, firstUser.Text,
				"the provider's user_prompt hook must preserve the complete positional message")
			require.Equal(t, firstWant, strings.TrimSpace(firstAssistant.Text),
				"the provider's turn_stop hook must expose the complete assistant message")
			firstSessionID, firstRunner := awaitSessionBound(
				t, h, first.RunnerID, first.TerminalSessionID, firstTap,
			)
			require.Equal(t, provider, firstRunner.ProviderID)
			require.True(t, h.eng.Terminal.SessionLive(ctx, first.TerminalSessionID),
				"the prompted provider must remain an interactive TUI after completing its turn")

			secondWant := fmt.Sprintf("CROWBAR-%s-SECOND", provider)
			secondText := "-p=Reply with only the exact text " + secondWant
			second, err := h.app.Usecases.Agent.SubmitPrompt(ctx, chatID, secondText, uuid.NewString())
			require.NoError(t, err)
			require.NotEqual(t, first.RunnerID, second.RunnerID)
			require.NotEqual(t, first.TerminalSessionID, second.TerminalSessionID)
			require.False(t, h.eng.Terminal.SessionLive(ctx, first.TerminalSessionID))

			secondTap := kit.AttachPTY(t, h.eng.Terminal, second.TerminalSessionID)
			t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), second.TerminalSessionID) })
			secondUser, secondAssistant := awaitPositionalPromptTurn(
				t, h, chatID, provider, firstAssistant.Sequence, secondText,
			)
			require.Equal(t, secondText, secondUser.Text)
			require.Equal(t, secondWant, strings.TrimSpace(secondAssistant.Text))
			secondSessionID, secondRunner := awaitSessionBound(
				t, h, second.RunnerID, second.TerminalSessionID, secondTap,
			)
			require.Equal(t, firstSessionID, secondSessionID,
				"the second prompted restart must resume the provider's native conversation")
			require.Equal(t, provider, secondRunner.ProviderID)
			require.True(t, h.eng.Terminal.SessionLive(ctx, second.TerminalSessionID),
				"Terminal fallback must still have a live native TUI after the resumed turn")
		})
	}
}

// awaitPositionalPromptTurn waits only on completed hook requests. It returns
// the exact user entry plus the first later assistant entry from the same
// provider. No PTY readiness marker or input is involved: seeing these entries
// is proof that the CLI accepted its argv prompt autonomously.
func awaitPositionalPromptTurn(
	t *testing.T,
	h *harness,
	chatID, provider string,
	after int,
	text string,
) (ledger.Message, ledger.Message) {
	t.Helper()
	type result struct {
		user      ledger.Message
		assistant ledger.Message
	}
	found := awaitHook(t, h, provider+" positional prompt turn", func() (result, bool) {
		page, err := h.app.Usecases.Agent.ReadMessages(context.Background(), chatID, after, 0, 200)
		if err != nil {
			return result{}, false
		}
		var user ledger.Message
		for _, message := range page.Items {
			if message.Sequence <= after || message.Provider != provider {
				continue
			}
			if user.Sequence == 0 {
				if message.Role == "user" && message.Text == text {
					user = message
				}
				continue
			}
			if message.Role == "assistant" && message.Sequence > user.Sequence && message.Text != "" {
				return result{user: user, assistant: message}, true
			}
		}
		return result{}, false
	})
	return found.user, found.assistant
}
