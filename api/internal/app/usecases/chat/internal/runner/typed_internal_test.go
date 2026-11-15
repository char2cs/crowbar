package runner

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/rivo/uniseg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// typingTerm is a PTY whose screen a test sets and whose writes it records.
type typingTerm struct {
	fakeTermForAttach
	screen   engineterminal.ScreenState
	writeErr error
	writes   []termWrite
}

type termWrite struct {
	sessionID string
	data      string
}

func (f *typingTerm) ScreenState(string) (engineterminal.ScreenState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.screen, nil
}

func (f *typingTerm) Write(_ context.Context, sessionID string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, termWrite{sessionID: sessionID, data: string(data)})
	return f.writeErr
}

func (f *typingTerm) written() []termWrite {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]termWrite(nil), f.writes...)
}

// plainTerm is a PTY that cannot report its screen state.
type plainTerm struct{ fakeTermForAttach }

// selectedConversations is reviveConversations with the chat's desired pick.
type selectedConversations struct {
	reviveConversations
	desired engineagents.Selection
}

func (c selectedConversations) ChatSelection(context.Context, string) (engineagents.Selection, error) {
	return c.desired, nil
}

// typedFixture is a chat whose idle claude CLI shows an empty composer.
type typedFixture struct {
	h    *spawnHarness
	term *typingTerm
	live engineagents.Runner
}

func newTypedFixture(t *testing.T, term seamTerm) typedFixture {
	t.Helper()
	h := newSpawnHarness(t, "")
	h.rs.term = term
	h.rs.home = func() (string, error) { return h.home, nil }
	h.rs.chats = stubChatsForSpawn{chat: domain.Chat{ID: h.chatID, WorkspaceID: "ws-1", ProviderID: "claude"}}
	h.rs.conversations = reviveConversations{provider: "claude"}
	h.rs.ledger = turnedTestLedger(t, "claude", "S1")
	runnerID, err := h.rs.spawnRunner(t.Context(), h.chatID, "ws-1", "claude", spawnInput{})
	require.NoError(t, err)
	live, ok := h.rs.live.byRunner(runnerID)
	require.True(t, ok)
	require.NotEmpty(t, live.TerminalSession)
	h.rs.conversations = selectedConversations{
		reviveConversations: reviveConversations{provider: "claude"},
		desired:             engineagents.Selection{PermissionLevel: live.LaunchPermissionLevel},
	}
	f := typedFixture{h: h, live: live}
	if typing, ok := term.(*typingTerm); ok {
		typing.mu.Lock()
		typing.writes = nil
		typing.mu.Unlock()
		f.term = typing
	}
	return f
}

// seamTerm is the PTY seam a fixture runs on.
type seamTerm interface {
	seam.TerminalCommander
	callCount() int
}

// emptyComposer is the screen of a claude CLI idle at its input box.
func emptyComposer(t *testing.T) engineterminal.ScreenState {
	t.Helper()
	typing, ok := testAgent(t, "claude").PromptType()
	require.True(t, ok, "the claude descriptor declares typed delivery")
	prefix := typing.ComposerRowPrefix
	return engineterminal.ScreenState{
		Text:      "Welcome\n\n" + prefix + "\n",
		CursorRow: 2, CursorCol: uniseg.StringWidth(prefix), BracketedPaste: true,
	}
}

func (f typedFixture) appendRows(t *testing.T, rows ...ledger.Row) {
	t.Helper()
	_, _, err := f.h.rs.ledger.Append(t.Context(), f.h.chatID, rows...)
	require.NoError(t, err)
}

// subagentOutlivingTurn is a finished main turn whose subagent still runs.
func (f typedFixture) subagentOutlivingTurn(t *testing.T) {
	t.Helper()
	started, err := ledger.NewRow(ledger.KindTurnStarted, "T-1", "", ledger.TurnStarted{RunnerID: f.live.ID})
	require.NoError(t, err)
	sub, err := ledger.NewRow(ledger.KindSubagent, "T-1", "S-1", ledger.SubagentBody{Phase: ledger.SubagentStart})
	require.NoError(t, err)
	ended, err := ledger.NewRow(ledger.KindTurnEnded, "T-1", "", ledger.TurnEnded{Status: string(ledger.TurnStatusDone)})
	require.NoError(t, err)
	f.appendRows(t, started, sub, ended)
	st := chatState(t, f.h.rs.ledger)
	_, running := st.RunningTurn()
	require.True(t, st.Busy() && !running, "precondition: only a subagent runs")
}

func pasted(text string) string { return "\x1b[200~" + text + "\x1b[201~\r" }

func TestSubmitPrompt_AnIdleCLIWithAnEmptyComposerIsTypedInto(t *testing.T) {
	for name, setup := range map[string]func(*testing.T, typedFixture){
		"idle": func(*testing.T, typedFixture) {},
		// Background work outliving its turn is what typing keeps alive.
		"only a subagent running": func(t *testing.T, f typedFixture) { t.Helper(); f.subagentOutlivingTurn(t) },
		"a faint suggestion after the cursor": func(t *testing.T, f typedFixture) {
			t.Helper()
			f.setScreen(func(s *engineterminal.ScreenState) { f.typeAfterCursor(s, "run the tests", true) })
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newTypedFixture(t, &typingTerm{screen: emptyComposer(t)})
			setup(t, f)
			requestID := uuid.NewString()

			got, err := f.h.rs.SubmitPrompt(t.Context(), f.h.chatID, "rename the helper", requestID)

			require.NoError(t, err)
			assert.Equal(t, domain.AgentPromptSubmission{RunnerID: f.live.ID, TerminalSessionID: f.live.TerminalSession}, got)
			assert.Equal(t, []termWrite{{sessionID: f.live.TerminalSession, data: pasted("rename the helper")}},
				f.term.written(), "one write: the paste and the submit key")
			p := promptReceipt(t, f.h.rs.ledger, requestID)
			assert.Equal(t, ledger.DeliveryType, p.Delivery)
			assert.Equal(t, f.live.ID, p.RunnerID)
			assert.Equal(t, ledger.PromptStatusPending, p.Status, "the CLI's hook is its receipt")
			assert.Equal(t, 1, f.term.callCount(), "no CLI is launched")
			assert.Empty(t, f.term.terminated, "the live CLI is not displaced")
		})
	}
}

// What the gate refuses goes where it went before: a restart, a steer, or a refusal.
func TestSubmitPrompt_ACLIThatCannotBeTypedIntoIsNotTyped(t *testing.T) {
	for name, tc := range map[string]struct {
		text     string
		setup    func(*testing.T, typedFixture)
		delivery string // the receipt's delivery; "" when the send is refused
		wantErr  error
	}{
		"a turn is running": {text: "and the tests", delivery: ledger.DeliverySteer, setup: func(t *testing.T, f typedFixture) {
			t.Helper()
			row, err := ledger.NewRow(ledger.KindTurnStarted, "T-1", "", ledger.TurnStarted{RunnerID: f.live.ID})
			require.NoError(t, err)
			f.appendRows(t, row)
		}},
		"another prompt is pending": {text: "next", wantErr: ErrPromptBusy, setup: func(t *testing.T, f typedFixture) {
			t.Helper()
			_, err := f.h.rs.recordPrompt(t.Context(), f.h.chatID, uuid.NewString(), "first", "", ledger.DeliveryRestart, f.live.ID, nil)
			require.NoError(t, err)
		}},
		"the selection changed": {text: "hello", delivery: ledger.DeliveryRestart, setup: func(t *testing.T, f typedFixture) {
			t.Helper()
			f.h.rs.conversations = selectedConversations{
				reviveConversations: reviveConversations{provider: "claude"},
				desired:             engineagents.Selection{Model: "opus", PermissionLevel: f.live.LaunchPermissionLevel},
			}
		}},
		"paste is off": {text: "hello", delivery: ledger.DeliveryRestart, setup: func(t *testing.T, f typedFixture) {
			t.Helper()
			f.setScreen(func(s *engineterminal.ScreenState) { s.BracketedPaste = false })
		}},
		"the composer holds a draft": {text: "hello", delivery: ledger.DeliveryRestart, setup: func(t *testing.T, f typedFixture) {
			t.Helper()
			f.setScreen(func(s *engineterminal.ScreenState) { s.CursorCol += len("half") })
		}},
		"the cursor sits before a draft": {text: "hello", delivery: ledger.DeliveryRestart, setup: func(t *testing.T, f typedFixture) {
			t.Helper()
			f.setScreen(func(s *engineterminal.ScreenState) { f.typeAfterCursor(s, "half", false) })
		}},
		"a slash command":            {text: "/compact", delivery: ledger.DeliveryRestart},
		"a shell command":            {text: "!ls", delivery: ledger.DeliveryRestart},
		"the end-paste sequence":     {text: "a\x1b[201~b", delivery: ledger.DeliveryRestart},
		"a pending move":             {text: "hello", wantErr: ErrChatMoving, setup: pendingMove},
		"a subagent and no type yet": {text: "/compact", wantErr: ErrPromptBusy, setup: subagentOnly},
	} {
		t.Run(name, func(t *testing.T) {
			f := newTypedFixture(t, &typingTerm{screen: emptyComposer(t)})
			if tc.setup != nil {
				tc.setup(t, f)
			}
			requestID := uuid.NewString()

			_, err := f.h.rs.SubmitPrompt(t.Context(), f.h.chatID, tc.text, requestID)

			assert.Empty(t, f.term.written(), "nothing is typed")
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.delivery, promptReceipt(t, f.h.rs.ledger, requestID).Delivery)
		})
	}
}

func subagentOnly(t *testing.T, f typedFixture) { t.Helper(); f.subagentOutlivingTurn(t) }

func pendingMove(t *testing.T, f typedFixture) {
	t.Helper()
	row, err := ledger.NewRow(ledger.KindMove, "", "", ledger.Move{Phase: ledger.MoveRequested, RequestID: "move-1", To: "ws-2"})
	require.NoError(t, err)
	f.appendRows(t, row)
	_, pending := chatState(t, f.h.rs.ledger).PendingMove()
	require.True(t, pending, "precondition: a move is pending")
}

// typeAfterCursor paints text on the composer row after the cursor, which stays put.
func (f typedFixture) typeAfterCursor(s *engineterminal.ScreenState, text string, faint bool) {
	rows := strings.Split(s.Text, "\n")
	rows[s.CursorRow] += text
	s.Text = strings.Join(rows, "\n")
	s.Faint = make([][]bool, len(rows))
	s.Faint[s.CursorRow] = make([]bool, s.CursorCol+len(text))
	for col := s.CursorCol; col < len(s.Faint[s.CursorRow]); col++ {
		s.Faint[s.CursorRow][col] = faint
	}
}

func (f typedFixture) setScreen(edit func(*engineterminal.ScreenState)) {
	f.term.mu.Lock()
	defer f.term.mu.Unlock()
	edit(&f.term.screen)
}

// Before typed delivery, a prompt with only a subagent running was refused as
// busy; a provider that declares no typing still is.
func TestSubmitPrompt_ASubagentAloneStillRefusesAnUntypedPrompt(t *testing.T) {
	f := newTypedFixture(t, &plainTerm{})
	f.subagentOutlivingTurn(t)

	_, err := f.h.rs.SubmitPrompt(t.Context(), f.h.chatID, "hello", uuid.NewString())

	require.ErrorIs(t, err, ErrPromptBusy)
}

func TestSubmitPrompt_APTYThatCannotReportItsScreenRestarts(t *testing.T) {
	f := newTypedFixture(t, &plainTerm{})
	requestID := uuid.NewString()

	_, err := f.h.rs.SubmitPrompt(t.Context(), f.h.chatID, "hello", requestID)

	require.NoError(t, err)
	assert.Equal(t, ledger.DeliveryRestart, promptReceipt(t, f.h.rs.ledger, requestID).Delivery)
}

func TestSubmitPrompt_AFailedTypeSettlesThePrompt(t *testing.T) {
	f := newTypedFixture(t, &typingTerm{screen: emptyComposer(t), writeErr: errors.New("pty closed")})
	requestID := uuid.NewString()

	_, err := f.h.rs.SubmitPrompt(t.Context(), f.h.chatID, "hello", requestID)

	require.ErrorIs(t, err, ErrPromptOutcomeUnknown)
	p := promptReceipt(t, f.h.rs.ledger, requestID)
	assert.Equal(t, ledger.PromptStatusSettled, p.Status)
	assert.Equal(t, ledger.ReasonDeliveryFailed, p.Reason)
	assert.Equal(t, 1, f.term.callCount(), "a failed type is not retried as a restart")
}

func TestSubmitPrompt_AResendIsNeverTypedTwice(t *testing.T) {
	f := newTypedFixture(t, &typingTerm{screen: emptyComposer(t)})
	requestID := uuid.NewString()

	_, err := f.h.rs.SubmitPrompt(t.Context(), f.h.chatID, "hello", requestID)
	require.NoError(t, err)
	_, err = f.h.rs.SubmitPrompt(t.Context(), f.h.chatID, "hello", requestID)
	require.NoError(t, err)

	assert.Len(t, f.term.written(), 1)
}

// The typed text is what a launch would be handed: an attachment the message
// opens with is escaped from the shell sigil and points at its stored file.
func TestSubmitPrompt_ATypedAttachmentIsGuardedAndMaterialised(t *testing.T) {
	f := newTypedFixture(t, &typingTerm{screen: emptyComposer(t)})
	dir := worktreepath.AttachmentsDir(f.h.home, f.h.chatID)
	name, _, err := repoattachments.Store(dir, "ab12", "a.png", []byte("png"))
	require.NoError(t, err)
	stored := filepath.ToSlash(filepath.Join(dir, name))
	text := "![shot](chats/" + f.h.chatID + "/attachments/" + name + ") what is this"

	_, err = f.h.rs.SubmitPrompt(t.Context(), f.h.chatID, text, uuid.NewString())

	require.NoError(t, err)
	require.Len(t, f.term.written(), 1)
	assert.Equal(t, pasted(" ![shot]("+stored+") what is this"), f.term.written()[0].data)
}

// A runner whose view is an attach of its api session is not typed into: its
// own CLI is the connection, which the api path delivers to.
func TestSubmitPromptTyped_ARunnerOverAnAPIConnectionOrAttachedViewIsNotTyped(t *testing.T) {
	for name, setup := range map[string]func(*testing.T, typedFixture){
		"api connection": func(t *testing.T, f typedFixture) { t.Helper(); f.h.seedLiveAPIConn(t, f.live.ID) },
		"attached view": func(_ *testing.T, f typedFixture) {
			f.h.rs.attached.set(f.live.ID, attachedView{termSessID: "attach-view"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newTypedFixture(t, &typingTerm{screen: emptyComposer(t)})
			setup(t, f)
			chat := domain.Chat{ID: f.h.chatID}

			_, handled, err := f.h.rs.submitPromptTyped(t.Context(), chat, uuid.NewString(), f.live,
				testAgent(t, "claude"), "hello")

			require.NoError(t, err)
			assert.False(t, handled)
			assert.Empty(t, f.term.written())
		})
	}
}
