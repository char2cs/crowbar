//go:build integration

package agent_test

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

const backstop = 5 * time.Minute

type hookBarrier struct {
	sig *kit.Signal
}

func newHookBarrier() *hookBarrier {
	return &hookBarrier{sig: kit.NewSignal()}
}

func (b *hookBarrier) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if c.Request.Method == "POST" && strings.HasSuffix(c.FullPath(), "/hooks/:provider/:event") {
			b.sig.Fire()
		}
	}
}

func awaitHook[T any](
	t *testing.T,
	h *harness,
	what string,
	pred func() (T, bool),
) T {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), backstop)
	defer cancel()
	return kit.Await(t, ctx, what, func() (T, bool) {
		h.app.Repositories.WaitQuiescent()
		return pred()
	}, h.hooks.sig)
}

var trustNeedle = map[string]string{
	"claude": "Enter to confirm",
	"codex":  "Press enter to continue",
}

const claudeTrustYesMarker = "Yes, I trust this folder"

const claudeTrustNoMarker = "No, exit"

var claudeTrustSelectionRe = regexp.MustCompile(
	`❯\s*(` + regexp.QuoteMeta(claudeTrustNoMarker) + `|` + regexp.QuoteMeta(claudeTrustYesMarker) + `)`)

func claudeSelectedYes(
	tap *kit.PTYTap,
) bool {
	rows := claudeTrustSelectionRe.FindAllStringSubmatch(kit.Readable(tap.Screen()), -1)
	return len(rows) > 0 && rows[len(rows)-1][1] == claudeTrustYesMarker
}

var codexModelRowRe = regexp.MustCompile(`model:\s+(\S+)\s+/model to change`)

func codexReady(
	tap *kit.PTYTap,
) bool {
	rows := codexModelRowRe.FindAllStringSubmatch(kit.Readable(tap.Screen()), -1)
	return len(rows) > 0 && rows[len(rows)-1][1] != "loading"
}

func (h *harness) firstOfProvider(
	provider string,
) bool {
	if h.trusted[provider] {
		return false
	}
	h.trusted[provider] = true
	return true
}

const resumeNeedle = "Conversation interrupted"

const claudeReadyNeedle = "shift+tab to cycle"

func settleCLI(
	t *testing.T,
	h *harness,
	tap *kit.PTYTap,
	termSessID string,
	provider string,
	runnerID string,
) {
	t.Helper()

	needle := ""
	switch {
	case h.firstOfProvider(provider):
		needle = trustNeedle[provider]
	case provider == "codex":
		needle = resumeNeedle
	default:
		awaitComposer(t, h, tap, termSessID, provider, "while coming back up on --resume")
		return
	}
	if needle == "" {
		t.Fatalf("settleCLI: no needle known for provider %q", provider)
	}

	ctx, cancel := context.WithTimeout(context.Background(), backstop)
	defer cancel()

	const (
		sawNeedle = "needle"
		sawBound  = "already-past"
		sawReady  = "ready-no-modal"
	)
	outcome := kit.Await(t, ctx, provider+" to paint "+needle,
		func() (string, bool) {
			if tap.Contains(needle) {
				return sawNeedle, true
			}
			if provider == "codex" && codexReady(tap) {
				return sawReady, true
			}
			if provider == "claude" {
				if r, ok := runnerByID(h, runnerID); ok &&
					announcedSession(h, r) != "" {
					return sawBound, true
				}
			}
			requireCLIAlive(t, h, tap, termSessID, provider, "while starting up")
			return "", false
		}, tap.Signal(), h.hooks.sig)

	switch {
	case outcome != sawNeedle:
	case provider == "codex":
		acknowledgeCodexDialog(t, h, tap, termSessID, needle)
	default:
		acknowledgeClaudeTrust(t, h, tap, termSessID)
	}
	if provider == "codex" {
		awaitCodexReady(t, h, tap, termSessID)
		return
	}
	if provider != "claude" {
		return
	}
	awaitComposer(t, h, tap, termSessID, provider, "while mounting its composer")
}

func awaitComposer(
	t *testing.T,
	h *harness,
	tap *kit.PTYTap,
	termSessID string,
	provider string,
	when string,
) {
	t.Helper()
	if provider != "claude" {
		t.Fatalf("awaitComposer: only claude's composer needle (%q) is known; %q needs its own "+
			"ready needle rather than this wait, which would hang until the backstop",
			claudeReadyNeedle, provider)
	}
	ctx, cancel := context.WithTimeout(context.Background(), backstop)
	defer cancel()
	kit.Await(t, ctx, provider+" to paint its composer (ready for input)",
		func() (bool, bool) {
			if tap.Contains(claudeReadyNeedle) {
				return true, true
			}
			requireCLIAlive(t, h, tap, termSessID, provider, when)
			return false, false
		}, tap.Signal())
}

const codexKeystrokeRetry = 2 * time.Second

const claudeTrustSettle = 2 * time.Second

func acknowledgeClaudeTrust(
	t *testing.T,
	h *harness,
	tap *kit.PTYTap,
	termSessID string,
) {
	t.Helper()
	deadline := time.Now().Add(backstop)
	for {
		if tap.Contains(claudeReadyNeedle) {
			return
		}
		requireCLIAlive(t, h, tap, termSessID, "claude",
			"while its trust dialog was being answered (an Enter on \"No, exit\" quits)")

		key, what := []byte("\x1b[B"), "move the trust-dialog selection"
		if claudeSelectedYes(tap) {
			key, what = []byte("\r"), "confirm trust"
		}
		if err := h.eng.Terminal.Write(context.Background(), termSessID, key); err != nil {
			t.Fatalf("settleCLI: %s: %v", what, err)
		}
		settleClaudeTrustKey(tap, deadline)

		if time.Now().After(deadline) {
			t.Fatalf("settleCLI: claude never left its trust dialog. Its screen was:\n%s",
				kit.Readable(tap.Screen()))
		}
	}
}

func settleClaudeTrustKey(
	tap *kit.PTYTap,
	deadline time.Time,
) {
	for {
		if tap.Contains(claudeReadyNeedle) {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		if !paintedSince(tap, tap.Mark(), claudeTrustSettle) {
			return
		}
	}
}

func acknowledgeCodexDialog(
	t *testing.T,
	h *harness,
	tap *kit.PTYTap,
	termSessID string,
	needle string,
) {
	t.Helper()
	deadline := time.Now().Add(backstop)
	for {
		mark := tap.Mark()
		if err := h.eng.Terminal.Write(context.Background(), termSessID, []byte("\r")); err != nil {
			t.Fatalf("settleCLI: acknowledge codex's %q: %v", needle, err)
		}
		if paintedSince(tap, mark, codexKeystrokeRetry) {
			return
		}
		requireCLIAlive(t, h, tap, termSessID, "codex", "while its dialog was being acknowledged")
		if time.Now().After(deadline) {
			t.Fatalf("settleCLI: codex never reacted to an Enter on its %q dialog. Its screen was:\n%s",
				needle, kit.Readable(tap.Screen()))
		}
	}
}

func paintedSince(
	tap *kit.PTYTap,
	mark int,
	within time.Duration,
) bool {
	expiry := time.After(within)
	for {
		edge := tap.Signal().Wait()
		if tap.Mark() > mark {
			return true
		}
		select {
		case <-edge:
		case <-expiry:
			return tap.Mark() > mark
		}
	}
}

func awaitCodexReady(
	t *testing.T,
	h *harness,
	tap *kit.PTYTap,
	termSessID string,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), backstop)
	defer cancel()
	kit.Await(t, ctx, "codex to finish starting up (ready to submit input)",
		func() (bool, bool) {
			if codexReady(tap) {
				return true, true
			}
			requireCLIAlive(t, h, tap, termSessID, "codex", "while finishing startup")
			return false, false
		}, tap.Signal())
}

func requireCLIAlive(
	t *testing.T,
	h *harness,
	tap *kit.PTYTap,
	termSessID string,
	provider string,
	when string,
) {
	t.Helper()
	if h.eng.Terminal.SessionLive(context.Background(), termSessID) {
		return
	}
	t.Fatalf("the %s CLI EXITED %s. It did not run slowly — it DIED, and no barrier it was "+
		"being waited on could ever have been satisfied. Its screen was:\n%s",
		provider, when, kit.Readable(tap.Screen()))
}

func spawnReady(
	t *testing.T,
	h *harness,
	wsID string,
	provider string,
) (chatID, runnerID, termSessID string, tap *kit.PTYTap) {
	t.Helper()
	ctx := context.Background()

	chatID, runnerID, err := spawnChat(ctx, h, wsID, provider)
	if err != nil {
		t.Fatalf("spawnReady: spawnChat(%s): %v", provider, err)
	}
	termSessID = liveRunnerTerminalSession(t, h, chatID)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), termSessID) })

	tap = kit.AttachPTY(t, h.eng.Terminal, termSessID)
	settleCLI(t, h, tap, termSessID, provider, runnerID)
	return chatID, runnerID, termSessID, tap
}

func attachReady(
	t *testing.T,
	h *harness,
	termSessID string,
	provider string,
	runnerID string,
) *kit.PTYTap {
	t.Helper()
	tap := kit.AttachPTY(t, h.eng.Terminal, termSessID)
	settleCLI(t, h, tap, termSessID, provider, runnerID)
	return tap
}

func drive(
	t *testing.T,
	h *harness,
	tap *kit.PTYTap,
	termSessID string,
	text string,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), backstop)
	defer cancel()

	mark := tap.Mark()
	require.NoError(t, h.eng.Terminal.Write(ctx, termSessID, []byte(text)), "drive: write text")

	needle := kit.EchoNeedle(text)
	kit.Await(t, ctx, "the CLI to echo the typed prompt back into its composer", func() (bool, bool) {
		if kit.ScreenContains(tap.ScreenSince(mark), needle) {
			return true, true
		}
		requireCLIAlive(t, h, tap, termSessID, "driven", "while the prompt was being typed into it")
		return false, false
	}, tap.Signal())

	require.NoError(t, h.eng.Terminal.Write(ctx, termSessID, []byte("\r")), "drive: submit")
}

func awaitSessionBound(
	t *testing.T,
	h *harness,
	runnerID string,
	termSessID string,
	tap *kit.PTYTap,
) (string, agents.Runner) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), backstop)
	defer cancel()

	var last agents.Runner
	sid := kit.Await(t, ctx, "runner "+runnerID+" to bind a provider conversation", func() (string, bool) {
		h.app.Repositories.WaitQuiescent()
		if runner, ok := runnerByID(h, runnerID); ok {
			last = runner
			if sid := announcedSession(h, runner); sid != "" {
				return sid, true
			}
		}
		if !h.eng.Terminal.SessionLive(context.Background(), termSessID) {
			t.Fatalf("awaitSessionBound: the CLI on runner %s EXITED without ever binding a conversation. "+
				"It did not run slowly — it DIED. Its screen was:\n%s",
				runnerID, kit.Readable(tap.Screen()))
		}
		return "", false
	}, h.hooks.sig, tap.Signal())
	return sid, last
}

func announcedSession(h *harness, r agents.Runner) string {
	snap, err := h.app.Usecases.AgentChat.ChatSnapshot(context.Background(), r.CurrentChatID)
	if err != nil {
		return ""
	}
	c, ok := snap.Conversation()
	if !ok || c.Provider != r.ProviderID || c.State != ledger.ConversationBound {
		return ""
	}
	if r.ProviderID == "claude" && r.ReportedModel == "" {
		return ""
	}
	return c.SessionID
}

func awaitAssistantReply(
	t *testing.T,
	h *harness,
	wsID string,
	chatID string,
	provider string,
	want string,
) []string {
	t.Helper()
	var texts []string
	awaitHook(t, h, provider+" to append an assistant turn containing "+want, func() (bool, bool) {
		texts = assistantReplies(readLedgerTurns(t, h, wsID, chatID), provider)
		for _, tx := range texts {
			if strings.Contains(tx, want) {
				return true, true
			}
		}
		return false, false
	})
	return texts
}

func awaitHandoffContains(
	t *testing.T,
	h *harness,
	chatID string,
	want string,
) string {
	t.Helper()
	return awaitHook(t, h, "the ledger to carry "+want, func() (string, bool) {
		blob, err := h.app.Usecases.AgentChat.AssembleHandoff(context.Background(), chatID)
		if err != nil {
			return "", false
		}
		return blob, strings.Contains(blob, want)
	})
}

func awaitTurnComplete(
	t *testing.T,
	h *harness,
	wsID string,
	chatID string,
	provider string,
) {
	t.Helper()
	awaitHook(t, h, provider+" to finish its turn (an assistant entry, and the chat no longer working)",
		func() (bool, bool) {
			if len(assistantReplies(readLedgerTurns(t, h, wsID, chatID), provider)) == 0 {
				return false, false
			}
			snap, err := h.app.Usecases.AgentChat.ChatSnapshot(context.Background(), chatID)
			if err != nil {
				return false, false
			}
			return !snap.Ledger.Busy, !snap.Ledger.Busy
		})
}
