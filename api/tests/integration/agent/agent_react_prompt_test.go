//go:build integration

package agent_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/tests/kit"
)

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
				_, _ = awaitSessionBound(t, h, idleRunnerID, idleTermID, idleTap)
			}

			firstWant := fmt.Sprintf("CROWBAR-%s-FIRST", provider)
			firstText := "--crowbar-leading-dash=Reply with only the exact text " + firstWant
			first, err := h.app.Usecases.AgentRunner.SubmitPrompt(ctx, chatID, firstText, uuid.NewString(), "", nil)
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
			second, err := h.app.Usecases.AgentRunner.SubmitPrompt(ctx, chatID, secondText, uuid.NewString(), "", nil)
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

func awaitPositionalPromptTurn(
	t *testing.T,
	h *harness,
	chatID, provider string,
	after int,
	text string,
) (domain.LedgerMessage, domain.LedgerMessage) {
	t.Helper()
	type result struct {
		user      domain.LedgerMessage
		assistant domain.LedgerMessage
	}
	found := awaitHook(t, h, provider+" positional prompt turn", func() (result, bool) {
		page, err := h.app.Usecases.AgentChat.ReadMessages(context.Background(), chatID, after, 0, 200)
		if err != nil {
			return result{}, false
		}
		var user domain.LedgerMessage
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
			// An assistant message is NOT the end of the turn. The streaming path
			// records one message per assistant message, so a model that narrates
			// and then keeps working banks a message mid-flight — and the caller
			// goes on to submit the next prompt, which the runner correctly refuses
			// with ErrPromptBusy because the turn really is still open. Wait for the
			// chat to stop working too: that is the turn CLOSING.
			if message.Role == "assistant" && message.Sequence > user.Sequence && message.Text != "" {
				if chatWorking(t, h, chatID) {
					return result{}, false
				}
				return result{user: user, assistant: message}, true
			}
		}
		return result{}, false
	})
	return found.user, found.assistant
}

// A prompt sent while claude is mid-turn is delivered into that SAME process by
// the Stop hook's reply: no respawn, one ledger row, in order.
func TestAgent_ReactPromptMidTurnSteersTheRunningCLI(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)
	ctx := context.Background()

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "react-steer-claude", repoPath)
	chatID, idleRunnerID, idleTermID, idleTap := spawnReady(t, h, wsID, "claude")
	_, _ = awaitSessionBound(t, h, idleRunnerID, idleTermID, idleTap)

	firstText := "Write the numbers 1 to 700 separated by commas in one single message, then the word CROWBAR-FIRST."
	first, err := h.app.Usecases.AgentRunner.SubmitPrompt(ctx, chatID, firstText, uuid.NewString(), "", nil)
	require.NoError(t, err)
	firstTap := kit.AttachPTY(t, h.eng.Terminal, first.TerminalSessionID)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), first.TerminalSessionID) })
	awaitHook(t, h, "the first turn is running", func() (bool, bool) {
		working := chatWorking(t, h, chatID)
		return working, working
	})

	secondText := "Now reply with only the exact text CROWBAR-SECOND"
	second, err := h.app.Usecases.AgentRunner.SubmitPrompt(ctx, chatID, secondText, uuid.NewString(), "", nil)
	require.NoError(t, err, "a prompt sent mid-turn must be accepted, not refused as busy")
	require.Equal(t, first.RunnerID, second.RunnerID, "the monitored CLI must not be replaced")
	require.Equal(t, first.TerminalSessionID, second.TerminalSessionID)

	type turns struct{ firstDone, user, answer domain.LedgerMessage }
	got := awaitHook(t, h, "the steered message and its answer", func() (turns, bool) {
		page, readErr := h.app.Usecases.AgentChat.ReadMessages(ctx, chatID, 0, 0, 200)
		if readErr != nil {
			return turns{}, false
		}
		var out turns
		for _, m := range page.Items {
			switch {
			case m.Role == "assistant" && strings.Contains(m.Text, "CROWBAR-FIRST"):
				out.firstDone = m
			case m.Role == "user" && m.Text == secondText:
				out.user = m
			case m.Role == "assistant" && strings.TrimSpace(m.Text) == "CROWBAR-SECOND":
				out.answer = m
			}
		}
		done := out.firstDone.Sequence > 0 && out.user.Sequence > out.firstDone.Sequence &&
			out.answer.Sequence > out.user.Sequence && !chatWorking(t, h, chatID)
		return out, done
	})
	require.NotZero(t, got.answer.Sequence)
	require.True(t, h.eng.Terminal.SessionLive(ctx, first.TerminalSessionID),
		"the CLI must still be the process that took the first prompt")
	requireCLIAlive(t, h, firstTap, first.TerminalSessionID, "claude", "after taking a steered prompt")
}
