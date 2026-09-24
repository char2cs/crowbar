//go:build integration && unix

package scripted_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

const healthy = "turn:\n  - say: continued\n"

// dormantSaying waits until chatID has no runner, no open turn, and a
// recorded reason — never a silent dormancy (S2, S3).
func (r *rig) dormantSaying(chatID string, reasons ...string) {
	r.t.Helper()
	r.eventually(func() bool {
		live, _ := r.live(chatID)
		return !live && !r.working(chatID) && slices.Contains(reasons, r.snapshot(chatID).Session.ExitReason)
	}, "the chat never went dormant with one of "+strings.Join(reasons, ", "))
}

// continues sends text and waits for the reply to land: the conversation
// carried on, on some rung (S5).
func (r *rig) continues(chatID, text string) {
	r.t.Helper()
	r.setScript(healthy)
	require.NoError(r.t, r.send(chatID, text))
	r.eventually(func() bool {
		return slices.Contains(r.said(chatID), "assistant: continued") && !r.working(chatID)
	}, "the conversation did not continue after "+text)
}

func (r *rig) firstTurn(chatID string) {
	r.t.Helper()
	require.NoError(r.t, r.send(chatID, "remember the word PELICAN"))
	r.eventually(func() bool { return len(r.said(chatID)) == 2 && !r.working(chatID) }, "first turn")
}

// A CLI that dies mid-turn closes its turn, says why, and the next message
// brings the conversation back.
func TestScripted_KillMidTurn(t *testing.T) {
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			r := newRig(t)
			chatID := r.spawn(provider)
			r.firstTurn(chatID)
			r.setScript("turn:\n  - tool: Bash\n  - crash: true\n")

			require.NoError(t, r.send(chatID, "this one dies"))

			r.dormantSaying(chatID, domain.AgentExitExited, domain.AgentExitConnectionLost)
			r.continues(chatID, "are you back?")
			assert.True(t, r.transcriptReached("PELICAN") || r.snapshot(chatID).Session.Rung == domain.AgentRungSession,
				"the revived CLI has the conversation: its own session, or the transcript")
		})
	}
}

// A CLI that stops producing mid-turn is stopped within a bound, leaves one
// interruption, and the chat takes the next message.
func TestScripted_HangThenStop(t *testing.T) {
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			r := newRig(t)
			chatID := r.spawn(provider)
			r.setScript("turn:\n  - hang: true\n")
			require.NoError(t, r.send(chatID, "hang now"))
			r.eventually(func() bool { return r.working(chatID) }, "the turn never opened")

			started := time.Now()
			require.NoError(t, r.runners().StopChat(context.Background(), chatID))
			assert.Less(t, time.Since(started), 15*time.Second, "Stop is bounded (S4)")

			r.eventually(func() bool { return !r.working(chatID) }, "the hung turn never closed")
			activity, err := r.d.app.Usecases.AgentTurn.ReadActivity(context.Background(), chatID, 0, 0)
			require.NoError(t, err)
			stopped := 0
			for _, in := range activity.Interruptions {
				if in.Kind == "stopped" {
					stopped++
				}
			}
			assert.Equal(t, 1, stopped, "exactly one stopped interruption")
			r.continues(chatID, "after the stop")
		})
	}
}

// A slow CLI holds its turn open without losing anything: a message sent
// meanwhile is refused as busy, not swallowed, and goes through afterwards.
func TestScripted_SlowHook(t *testing.T) {
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			r := newRig(t)
			chatID := r.spawn(provider)
			r.setScript("turn:\n  - slow_ms: 1500\n  - say: slowly\n")
			require.NoError(t, r.send(chatID, "take your time"))
			r.eventually(func() bool { return r.working(chatID) }, "the turn never opened")

			assert.ErrorIs(t, r.send(chatID, "are you done?"), agentusecase.ErrPromptBusy)

			r.eventually(func() bool {
				return slices.Contains(r.said(chatID), "assistant: slowly") && !r.working(chatID)
			}, "the slow turn never closed")
			r.continues(chatID, "now then")
		})
	}
}

// A dropped api socket ends its runner with a reason; the next message
// re-establishes the conversation.
func TestScripted_SocketDrop(t *testing.T) {
	r := newRig(t)
	chatID := r.spawn("codex")
	r.firstTurn(chatID)
	r.setScript("turn:\n  - drop: true\n")

	require.NoError(t, r.send(chatID, "drop the line"))

	r.dormantSaying(chatID, domain.AgentExitConnectionLost, domain.AgentExitExited)
	r.continues(chatID, "reconnect")
}

// A daemon restart mid-turn closes the turn, records why, and the next
// message continues the same conversation.
func TestScripted_DaemonRestartMidTurn(t *testing.T) {
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			r := newRig(t)
			chatID := r.spawn(provider)
			r.firstTurn(chatID)
			r.setScript("turn:\n  - hang: true\n")
			require.NoError(t, r.send(chatID, "interrupted by a restart"))
			r.eventually(func() bool { return r.working(chatID) }, "the turn never opened")

			r.restart()

			r.dormantSaying(chatID, domain.AgentExitDaemonRestart)
			r.continues(chatID, "after the restart")
			assert.Contains(t, r.said(chatID), "user: remember the word PELICAN", "the ledger survived")
		})
	}
}

// An ask nobody answers is answered for them when its budget runs out, and
// the turn goes on to close — it never holds the conversation forever.
func TestScripted_UnansweredPermissionAsk(t *testing.T) {
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			r := newRig(t)
			chatID := r.spawn(provider)
			r.setScript("turn:\n  - permission: Bash\n  - say: carried on\n")
			require.NoError(t, r.send(chatID, "needs approval"))
			r.eventually(func() bool { return len(r.choices(chatID)) == 1 }, "the ask never reached the chat")

			r.eventually(func() bool {
				return slices.Contains(r.said(chatID), "assistant: carried on") && !r.working(chatID)
			}, "an unanswered ask held the turn open")
			assert.Empty(t, r.choices(chatID), "the expired ask is closed")
			require.Len(t, r.logged("answer"), 1)
			assert.NotContains(t, r.logged("answer")[0]["verdict"], "allow",
				"an expired ask is never answered with an approval")
		})
	}
}

// An ask the user answers reaches the CLI as that answer.
func TestScripted_AnsweredPermissionAsk(t *testing.T) {
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			r := newRig(t)
			chatID := r.spawn(provider)
			r.setScript("turn:\n  - permission: Bash\n  - say: approved\n")
			require.NoError(t, r.send(chatID, "needs approval"))
			var choice domain.ActivityChoice
			r.eventually(func() bool {
				pending := r.choices(chatID)
				if len(pending) == 1 {
					choice = pending[0]
				}
				return len(pending) == 1
			}, "the ask never reached the chat")

			require.NoError(t, r.d.app.Usecases.AgentAnswer.AnswerChoice(
				context.Background(), chatID, choice.ID, []string{allowOption(choice)}, "", nil))

			r.eventually(func() bool { return slices.Contains(r.said(chatID), "assistant: approved") }, "turn")
			require.Len(t, r.logged("answer"), 1)
			verdict, _ := r.logged("answer")[0]["verdict"].(string)
			assert.Regexp(t, `allow|accept`, verdict)
		})
	}
}

func allowOption(c domain.ActivityChoice) string {
	for _, o := range c.Options {
		if strings.Contains(strings.ToLower(o.ID+o.Label), "allow") || strings.Contains(strings.ToLower(o.ID), "accept") {
			return o.ID
		}
	}
	return "allow"
}

// A vendor session the provider no longer has is never resumed: the chat
// continues on a fresh session handed Crowbar's own transcript, and says so.
func TestScripted_MissingSessionFileFallsToTheTranscript(t *testing.T) {
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			r := newRig(t)
			chatID := r.spawn(provider)
			r.firstTurn(chatID)
			require.NoError(t, r.runners().StopChat(context.Background(), chatID))
			r.dormantSaying(chatID, domain.AgentExitStopped)
			removeSessionFiles(t)

			r.continues(chatID, "do you remember?")

			assert.Empty(t, r.logged("refused"), "the lost session was never handed to the CLI")
			assert.Equal(t, domain.AgentRungTranscript, r.snapshot(chatID).Session.Rung)
			assert.True(t, r.transcriptReached("PELICAN"), "the fresh session was handed the conversation")
		})
	}
}

// A resume the CLI refuses although its session file exists (a changed CLI,
// a corrupt transcript) still continues the conversation, on the next rung.
func TestScripted_RefusedResumeContinuesOnTheTranscript(t *testing.T) {
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			r := newRig(t)
			chatID := r.spawn(provider)
			r.firstTurn(chatID)
			require.NoError(t, r.runners().StopChat(context.Background(), chatID))
			r.dormantSaying(chatID, domain.AgentExitStopped)
			r.setScript("resume: refuse\n" + healthy)

			require.NoError(t, r.send(chatID, "carry on please"))

			r.eventually(func() bool {
				return slices.Contains(r.said(chatID), "assistant: continued") && !r.working(chatID)
			}, "a refused resume stranded the chat")
			assert.NotEmpty(t, r.logged("refused"), "the CLI did refuse\n%s", r.dump())
			assert.Equal(t, domain.AgentRungTranscript, r.snapshot(chatID).Session.Rung)
			assert.True(t, r.transcriptReached("PELICAN"))
		})
	}
}

// transcriptReached reports whether a CLI after the first turn was handed
// text: in a relaunch's argv (claude's context), a codex call that sets up a
// thread (developer instructions, injected items), or a later prompt.
func (r *rig) transcriptReached(text string) bool {
	starts := r.logged("start")
	for _, e := range starts[1:] {
		argv, _ := e["argv"].([]any)
		for _, a := range argv {
			if s, _ := a.(string); strings.Contains(s, text) {
				return true
			}
		}
	}
	for _, e := range r.logged("rpc") {
		if s, _ := e["params"].(string); strings.Contains(s, text) && e["method"] != "turn/start" {
			return true
		}
	}
	for _, e := range r.logged("prompt")[1:] {
		if s, _ := e["text"].(string); strings.Contains(s, text) {
			return true
		}
	}
	return false
}

func removeSessionFiles(t *testing.T) {
	t.Helper()
	for _, root := range []string{os.Getenv("CLAUDE_CONFIG_DIR"), os.Getenv("CODEX_HOME")} {
		require.NoError(t, filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(path, ".jsonl") {
				return os.Remove(path)
			}
			return nil
		}))
	}
}
