//go:build integration && unix

package scripted_test

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

var providers = []string{"claude", "codex"}

// A whole turn — prompt, tool call, subagent, compaction, reply — lands in the
// ledger and closes, on both providers, over their shipped descriptors.
func TestScripted_AFullTurnLandsAndCloses(t *testing.T) {
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			r := newRig(t)
			r.setScript(`
turn:
  - tool: Bash
  - subagent: general-purpose
  - compact: true
  - say: all done
`)
			chatID := r.spawn(provider)
			require.NoError(t, r.send(chatID, "do the thing"))

			r.eventually(func() bool {
				return slices.Contains(r.said(chatID), "assistant: all done") && !r.working(chatID)
			}, "the turn never landed and closed")
			assert.Contains(t, r.said(chatID), "user: do the thing")
			activity, err := r.d.app.Usecases.AgentTurn.ReadActivity(context.Background(), chatID, 0, 0)
			require.NoError(t, err)
			require.NotEmpty(t, activity.ToolCalls, "the tool call was recorded")
			for _, call := range activity.ToolCalls {
				assert.NotNil(t, call.EndedAt, "every tool call closes: %+v", call)
			}
			for _, sub := range activity.Subagents {
				assert.NotNil(t, sub.EndedAt, "every subagent closes: %+v", sub)
			}
			snap := r.snapshot(chatID)
			assert.Equal(t, "live", snap.Phase)
		})
	}
}

// A second message continues the SAME vendor session: claude restarts per
// prompt and resumes it; codex keeps its connection.
func TestScripted_ASecondMessageContinuesTheSession(t *testing.T) {
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			r := newRig(t)
			chatID := r.spawn(provider)
			require.NoError(t, r.send(chatID, "first"))
			r.eventually(func() bool { return len(r.said(chatID)) == 2 && !r.working(chatID) }, "first turn")

			require.NoError(t, r.send(chatID, "second"))
			r.eventually(func() bool { return len(r.said(chatID)) == 4 && !r.working(chatID) }, "second turn")

			sessions := map[string]bool{}
			for _, e := range r.logged("prompt") {
				sessions[e["session"].(string)] = true
			}
			assert.Len(t, sessions, 1, "both prompts reached one vendor session")
			assert.Empty(t, r.logged("refused"))
			if provider == "claude" {
				assert.Equal(t, domain.AgentRungSession, r.snapshot(chatID).Session.Rung,
					"claude relaunched on its own session")
			} else {
				assert.Len(t, r.logged("start"), 1, "codex kept its one app-server")
			}
		})
	}
}

// A stopped chat's next message resumes the provider's OWN session — the
// first rung — on both providers. For codex this needs the thread it announces
// before its runner row commits to be kept, not dropped.
func TestScripted_ADormantChatResumesItsOwnSession(t *testing.T) {
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			r := newRig(t)
			chatID := r.spawn(provider)
			r.firstTurn(chatID)
			first := r.logged("prompt")[0]["session"]
			require.NoError(t, r.runners().StopChat(context.Background(), chatID))
			r.dormantSaying(chatID, domain.AgentExitStopped)

			r.continues(chatID, "welcome back")

			prompts := r.logged("prompt")
			assert.Equal(t, first, prompts[len(prompts)-1]["session"], "the same vendor session took the message")
			assert.Equal(t, domain.AgentRungSession, r.snapshot(chatID).Session.Rung)
		})
	}
}
