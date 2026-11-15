//go:build integration

package tests

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rivo/uniseg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/tests/kit"
)

// composerPrefix is the shipped claude descriptor's composer row prefix: the
// glyph and a NO-BREAK space.
const composerPrefix = "❯ "

// typedChat is a claude-fake chat whose CLI is a composer the daemon can type into.
type typedChat struct {
	h        *harness
	imported importedRepo
	fake     kit.FakeProvider
	chatID   string
	launch   kit.Launch
	session  string
}

func newTypedChat(t *testing.T, composer kit.Composer) typedChat {
	t.Helper()
	h := newHarness(t)
	c := typedChat{h: h, imported: importWritableWorkspace(t, h)}
	composer.Prefix = composerPrefix
	c.fake = kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude", Composer: &composer})
	c.chatID, c.launch = createFakeChat(t, h, c.imported, c.fake)
	c.session = argAfter(c.launch.Argv, "--session-id")
	require.NotEmpty(t, c.session)
	c.hook(t, "session_start", map[string]any{"source": "startup"})
	c.awaitCursor(t, uniseg.StringWidth(composerPrefix)+uniseg.StringWidth(composer.Draft))
	return c
}

func (c typedChat) hook(t *testing.T, event string, payload map[string]any) {
	t.Helper()
	payload["session_id"] = c.session
	postSessionHook(t, c.launch, c.h.url+repoBase(c.imported), "claude", event, payload)
}

// awaitCursor blocks until the daemon's own emulator has the composer painted,
// bracketed paste on and the cursor at col: what the gate reads.
func (c typedChat) awaitCursor(t *testing.T, col int) {
	t.Helper()
	sessionID := getAgentChat(t, c.h, repoBase(c.imported), c.chatID).TerminalSessionID
	require.NotEmpty(t, sessionID)
	screens, ok := c.h.eng.Terminal.(terminal.ScreenStateReader)
	require.True(t, ok, "the terminal engine reports screen state")
	tap := kit.AttachPTY(t, c.h.eng.Terminal, sessionID)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	kit.Await(t, ctx, "the composer painted", func() (terminal.ScreenState, bool) {
		st, err := screens.ScreenState(sessionID)
		return st, err == nil && st.BracketedPaste && st.CursorCol == col
	}, tap.Signal())
}

func (c typedChat) send(t *testing.T, text, requestID string) {
	t.Helper()
	require.Equal(t, http.StatusOK, postPrompt(t, c.h, c.imported, c.chatID, text, requestID))
}

func (c typedChat) pending(t *testing.T) []ledger.Prompt {
	t.Helper()
	return chatState(t, c.h, c.chatID).PendingPrompts()
}

// An idle prompt is typed into the running CLI, never relaunched with it, and
// the CLI's own prompt hook is its receipt.
func TestRegression_AnIdlePromptIsTypedIntoTheRunningCLI(t *testing.T) {
	c := newTypedChat(t, kit.Composer{})
	requestID := uuid.NewString()

	c.send(t, "rename the helper", requestID)

	assert.Equal(t, []string{"rename the helper"}, c.launch.AwaitSubmitted(t, 1))
	assert.Equal(t, []string{c.launch.RunnerID}, c.fake.LaunchedRunners(t), "the CLI is launched once")
	live := getAgentChat(t, c.h, repoBase(c.imported), c.chatID).LiveRunnerID
	assert.Equal(t, c.launch.RunnerID, live)
	assert.Equal(t, c.launch.PID, c.fake.AwaitLaunch(t, live).PID, "the same process took the prompt")
	pending := c.pending(t)
	require.Len(t, pending, 1)
	assert.Equal(t, ledger.DeliveryType, pending[0].Delivery)

	c.hook(t, "user_prompt", map[string]any{"prompt": "rename the helper"})

	assert.Empty(t, c.pending(t), "the hook delivered it")
	turn, running := chatState(t, c.h, c.chatID).RunningTurn()
	require.True(t, running)
	assert.Equal(t, requestID, turn.RequestID, "the typed prompt opened its own turn")
}

// A leading attachment is typed escaped and materialised, and its echo is
// still recognised as the prompt that was sent.
func TestRegression_ATypedAttachmentIsReceivedAsThePromptSent(t *testing.T) {
	c := newTypedChat(t, kit.Composer{})
	dir := worktreepath.AttachmentsDir(c.h.home, c.chatID)
	name, _, err := repoattachments.Store(dir, "ab12", "shot.png", []byte("png"))
	require.NoError(t, err)
	text := "![shot](chats/" + c.chatID + "/attachments/" + name + ") what is this"
	requestID := uuid.NewString()

	c.send(t, text, requestID)

	submitted := c.launch.AwaitSubmitted(t, 1)
	assert.Equal(t, []string{" ![shot](" + filepath.ToSlash(filepath.Join(dir, name)) + ") what is this"}, submitted)
	c.hook(t, "user_prompt", map[string]any{"prompt": submitted[0]})
	assert.Empty(t, c.pending(t))
	turn, _ := chatState(t, c.h, c.chatID).RunningTurn()
	assert.Equal(t, requestID, turn.RequestID)
}

// A box that holds a draft, or a command, is never typed over: the CLI is
// relaunched with the prompt as before.
func TestRegression_AnUntypeablePromptStillRestartsTheCLI(t *testing.T) {
	for name, tc := range map[string]struct {
		composer kit.Composer
		text     string
	}{
		"a draft in the box": {composer: kit.Composer{Draft: "half"}, text: "hello"},
		"a slash command":    {text: "/compact"},
	} {
		t.Run(name, func(t *testing.T) {
			c := newTypedChat(t, tc.composer)

			c.send(t, tc.text, uuid.NewString())

			require.Eventually(t, func() bool { return len(c.fake.LaunchedRunners(t)) == 2 },
				10*time.Second, 10*time.Millisecond, "the CLI is relaunched with the prompt")
			assert.Empty(t, c.launch.Submitted(t), "nothing was typed into the first CLI")
		})
	}
}

func TestRegression_AResentTypedPromptIsTypedOnce(t *testing.T) {
	c := newTypedChat(t, kit.Composer{})
	requestID := uuid.NewString()
	c.send(t, "once only", requestID)
	c.send(t, "once only", requestID)
	c.launch.AwaitSubmitted(t, 1)
	c.hook(t, "user_prompt", map[string]any{"prompt": "once only"})
	c.hook(t, "turn_stop", map[string]any{"last_assistant_message": "done"})

	// The next prompt is typed behind whatever the resend wrote, so it bounds it.
	c.send(t, "next", uuid.NewString())

	assert.Equal(t, []string{"once only", "next"}, c.launch.AwaitSubmitted(t, 2))
	assert.Len(t, c.fake.LaunchedRunners(t), 1)
}
