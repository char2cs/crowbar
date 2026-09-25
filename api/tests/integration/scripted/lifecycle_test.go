//go:build integration && unix

package scripted_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// A native turn whose close never comes — codex reports idle and sends no
// turn/completed — is closed by that idle report, not left spinning.
func TestScripted_NativeTurnWithoutACloseIsClosed(t *testing.T) {
	r := newRig(t)
	chatID := r.spawn("codex")
	r.setScript("turn:\n  - idle: true\n")
	require.NoError(t, r.send(chatID, "reason quietly"))
	r.eventually(func() bool { return r.working(chatID) }, "the turn never opened")

	r.eventually(func() bool { return !r.working(chatID) }, "a turn the provider called idle kept spinning")
	r.continues(chatID, "next")
}

// Switching away and back resumes the provider's OWN session, handed only
// what happened meanwhile; the provider switched to is handed the transcript.
func TestScripted_SwitchProviderRoundTrip(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	chatID := r.spawn("claude")
	r.firstTurn(chatID)
	claudeSession := r.logged("prompt")[0]["session"]

	_, err := r.runners().SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)
	r.continues(chatID, "codex, what was the word?")
	assert.True(t, r.transcriptReached("PELICAN"), "codex was handed the conversation")

	_, err = r.runners().SwitchProvider(ctx, chatID, "claude")
	require.NoError(t, err)
	require.NoError(t, r.send(chatID, "claude again"))
	r.eventually(func() bool {
		prompts := r.logged("prompt")
		last := prompts[len(prompts)-1]
		return last["cli"] == "claude" && last["session"] == claudeSession
	}, "claude did not resume its own session")
	r.eventually(func() bool { return !r.working(chatID) }, "the turn never closed")
}

// Deleting a chat mid-turn is bounded, and leaves nothing behind for it.
func TestScripted_DeleteMidTurn(t *testing.T) {
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			r := newRig(t)
			chatID := r.spawn(provider)
			r.setScript("turn:\n  - hang: true\n")
			require.NoError(t, r.send(chatID, "busy"))
			r.eventually(func() bool { return r.working(chatID) }, "the turn never opened")

			started := time.Now()
			require.NoError(t, r.d.app.Usecases.AgentChat.PurgeChat(context.Background(), chatID))
			assert.Less(t, time.Since(started), 15*time.Second, "a delete is bounded")

			all, err := r.d.app.Repositories.AgentRunner.AllLive(context.Background())
			require.NoError(t, err)
			for _, runner := range all {
				assert.NotEqual(t, chatID, runner.CurrentChatID, "a runner outlived its chat")
			}
			_, err = r.d.app.Usecases.AgentChat.ChatSnapshot(context.Background(), chatID)
			assert.Error(t, err, "the chat is gone")
		})
	}
}

// The compact intent reaches each provider as its own gesture and settles
// without a reply.
func TestScripted_CompactIntent(t *testing.T) {
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			r := newRig(t)
			chatID := r.spawn(provider)
			r.firstTurn(chatID)

			require.NoError(t, r.runners().Compact(context.Background(), chatID))

			r.eventually(func() bool {
				activity, err := r.d.app.Usecases.AgentTurn.ReadActivity(context.Background(), chatID, 0, 0)
				require.NoError(t, err)
				return slices.ContainsFunc(activity.Interruptions, func(in domain.ActivityInterruption) bool {
					return in.Kind == "compacted" || in.Kind == "compaction"
				})
			}, "the compaction was never recorded")
			r.eventually(func() bool { return !r.working(chatID) }, "a compaction left the chat busy")
			r.continues(chatID, "after compacting")
		})
	}
}

// Two chats in one worktree each keep their own vendor session: a revive is
// never "the latest session in this directory".
func TestScripted_TwoChatsInOneWorktreeKeepTheirOwnSessions(t *testing.T) {
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			r := newRig(t)
			ctx := context.Background()
			a := r.spawn(provider)
			r.firstTurn(a)
			b := r.spawn(provider)
			require.NoError(t, r.send(b, "I am chat B"))
			r.eventually(func() bool { return len(r.said(b)) == 2 && !r.working(b) }, "chat B's turn")
			sessionOf := map[string]any{}
			for _, e := range r.logged("prompt") {
				sessionOf[e["text"].(string)] = e["session"]
			}
			require.NotEqual(t, sessionOf["remember the word PELICAN"], sessionOf["I am chat B"])
			for _, chat := range []string{a, b} {
				require.NoError(t, r.runners().StopChat(ctx, chat))
			}

			require.NoError(t, r.send(a, "A again"))
			r.eventually(func() bool { return len(r.said(a)) == 4 && !r.working(a) }, "chat A's revive")
			require.NoError(t, r.send(b, "B again"))
			r.eventually(func() bool { return len(r.said(b)) == 4 && !r.working(b) }, "chat B's revive")

			for _, e := range r.logged("prompt") {
				sessionOf[e["text"].(string)] = e["session"]
			}
			assert.Equal(t, sessionOf["remember the word PELICAN"], sessionOf["A again"], "A resumed A's session")
			assert.Equal(t, sessionOf["I am chat B"], sessionOf["B again"], "B resumed B's session")
		})
	}
}

// A prompt typed straight into claude's terminal is recorded like a sent one.
func TestScripted_TypedPromptInTheTerminal(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	chatID := r.spawn("claude")
	r.eventually(func() bool { return len(r.logged("start")) > 0 }, "claude never started")
	live, err := r.runners().LiveRunnerForChat(ctx, chatID)
	require.NoError(t, err)
	r.setScript("turn:\n  - say: typed reply\n")

	require.NoError(t, r.d.eng.Terminal.Write(ctx, live.TerminalSession, []byte("typed by hand\r")))

	r.eventually(func() bool {
		return slices.Contains(r.said(chatID), "user: typed by hand") &&
			slices.Contains(r.said(chatID), "assistant: typed reply") && !r.working(chatID)
	}, "the typed turn never landed")
}
