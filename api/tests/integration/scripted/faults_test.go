//go:build integration && unix

package scripted_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/tests/kit"
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
// carried on (S5).
func (r *rig) continues(chatID, text string) {
	r.t.Helper()
	r.setScript(healthy)
	require.NoError(r.t, r.send(chatID, text))
	r.eventually(func() bool {
		said := r.said(chatID)
		asked := slices.Index(said, "user: "+text)
		return asked >= 0 && slices.Contains(said[asked:], "assistant: continued") && !r.working(chatID)
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
			prompts := r.logged("prompt")
			assert.Equal(t, prompts[0]["session"], prompts[len(prompts)-1]["session"],
				"the revived CLI resumed the chat's own session")
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

// A dropped api connection ends its runner with a reason; the next message
// re-establishes the conversation.
func TestScripted_ConnectionDrop(t *testing.T) {
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

// An ask over the 128 KiB a held ask may carry is refused on its api
// connection at once: it never waits on the chat, and nothing approves it.
func TestScripted_AnAskTooBigToHoldIsRefused(t *testing.T) {
	r := newRig(t)
	chatID := r.spawn("codex")
	r.setScript("turn:\n  - permission: " + strings.Repeat("x", 129<<10) + "\n  - say: carried on\n")
	require.NoError(t, r.send(chatID, "needs approval"))

	r.eventually(func() bool {
		assert.Empty(t, r.choices(chatID), "an ask Crowbar cannot hold is never pending")
		return slices.Contains(r.said(chatID), "assistant: carried on") && !r.working(chatID)
	}, "the refused ask held the turn open")
	assert.Empty(t, r.choices(chatID))
	require.Len(t, r.logged("answer"), 1)
	assert.NotRegexp(t, `allow|accept`, r.logged("answer")[0]["verdict"])
	activity, err := r.d.app.Usecases.AgentTurn.ReadActivity(context.Background(), chatID, 0, 0)
	require.NoError(t, err)
	require.Len(t, activity.Choices, 1)
	assert.Equal(t, ledger.ResolutionRefused, activity.Choices[0].Resolution)
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

// A vendor session the provider no longer has is refused by its CLI: the chat
// loses it, and the next message starts a fresh session with no transcript.
func TestScripted_MissingSessionFileIsLost(t *testing.T) {
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			r := newRig(t)
			chatID := r.spawn(provider)
			r.firstTurn(chatID)
			require.NoError(t, r.runners().StopChat(context.Background(), chatID))
			r.dormantSaying(chatID, domain.AgentExitStopped)
			r.removeSessionFiles()

			r.lostThenFresh(chatID, provider, "do you remember?")
		})
	}
}

// A resume the CLI refuses although its session file exists (a changed CLI,
// a corrupt transcript) is lost the same way.
func TestScripted_RefusedResumeIsLost(t *testing.T) {
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			r := newRig(t)
			chatID := r.spawn(provider)
			r.firstTurn(chatID)
			require.NoError(t, r.runners().StopChat(context.Background(), chatID))
			r.dormantSaying(chatID, domain.AgentExitStopped)
			r.setScript("resume: refuse\n" + healthy)

			r.lostThenFresh(chatID, provider, "carry on please")
		})
	}
}

// lostThenFresh sends text to chatID, whose session its CLI refuses: the
// message is not taken, and the next one starts a fresh session handed no
// transcript of the conversation.
func (r *rig) lostThenFresh(chatID, provider, text string) {
	r.t.Helper()
	first := r.logged("prompt")[0]["session"]
	r.sendToALostSession(chatID, provider, text)
	assert.NotEmpty(r.t, r.logged("refused"), "the CLI refused the session")

	r.continues(chatID, "start again")

	prompts := r.logged("prompt")
	assert.NotEqual(r.t, first, prompts[len(prompts)-1]["session"], "the refused session is never resumed again")
	assert.False(r.t, r.transcriptReached("PELICAN"), "a lost session's replacement is handed no transcript\n%s", r.dump())
	assert.Equal(r.t, 1, r.lostSessions(chatID), "the fresh session is not lost")
}

// sendToALostSession sends text to chatID, whose session its CLI refuses: the
// prompt is settled session_lost and the session is lost once. Codex's chat
// runs over its api connection, which refuses before the send answers.
func (r *rig) sendToALostSession(chatID, provider, text string) {
	r.t.Helper()
	requestID := uuid.NewString()
	_, err := r.runners().SubmitPrompt(context.Background(), chatID, text, requestID, "", nil)
	if provider == "codex" || err != nil {
		require.ErrorIs(r.t, err, agentusecase.ErrPromptOutcomeUnknown)
	}
	r.eventually(func() bool {
		c, ok := r.snapshot(chatID).Conversation()
		return ok && c.State == ledger.ConversationLost && !r.working(chatID) && len(r.settled(chatID, requestID)) > 0
	}, "the CLI never refused the session")
	assert.Equal(r.t, []string{ledger.ReasonSessionLost}, r.settled(chatID, requestID), "the refused prompt was not taken")
	assert.Equal(r.t, 1, r.lostSessions(chatID))
}

// settled is every reason chatID's ledger settled requestID's prompt with.
func (r *rig) settled(chatID, requestID string) []string {
	r.t.Helper()
	var bodies []string
	require.NoError(r.t, r.d.adapters.ChatLedgerDB().Raw(
		"SELECT body FROM chat_ledger WHERE chat_id = ? AND kind = ? ORDER BY seq",
		chatID, string(ledger.KindPromptSettled)).Scan(&bodies).Error)
	var reasons []string
	for _, body := range bodies {
		var s ledger.PromptSettled
		require.NoError(r.t, json.Unmarshal([]byte(body), &s))
		if s.RequestID == requestID {
			reasons = append(reasons, s.Reason)
		}
	}
	return reasons
}

// lostSessions counts chatID's session_lost rows.
func (r *rig) lostSessions(chatID string) int {
	r.t.Helper()
	var n int
	require.NoError(r.t, r.d.adapters.ChatLedgerDB().Raw(
		"SELECT count(*) FROM chat_ledger WHERE chat_id = ? AND kind = ?",
		chatID, string(ledger.KindSessionLost)).Scan(&n).Error)
	return n
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

// removeSessionFiles deletes every transcript in the CLIs' homes, which must
// lie inside the rig's temp dir.
func (r *rig) removeSessionFiles() {
	r.t.Helper()
	for _, env := range []string{kit.ClaudeHomeEnv, kit.CodexHomeEnv} {
		root := kit.RequireProviderHomeIn(r.t, env, r.providers)
		require.NoError(r.t, filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(path, ".jsonl") {
				return os.Remove(path)
			}
			return nil
		}))
	}
}
