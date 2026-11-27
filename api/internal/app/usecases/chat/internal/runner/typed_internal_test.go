package runner

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

type plainTerm struct{ fakeTermForAttach }

type selectedConversations struct {
	reviveConversations
	desired engineagents.Selection
}

func (c selectedConversations) ChatSelection(context.Context, string) (engineagents.Selection, error) {
	return c.desired, nil
}

type typedFixture struct {
	h    *spawnHarness
	term *typingTerm
	live engineagents.Runner
}

func newTypedFixture(t *testing.T, term seamTerm) typedFixture {
	t.Helper()
	h := newSpawnHarness(t, "")
	h.rs.term = term
	h.rs.pause = func(context.Context, time.Duration) error { return nil }
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

type seamTerm interface {
	seam.TerminalCommander
	callCount() int
}

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

func (f typedFixture) typedText() string {
	var b strings.Builder
	writes := f.term.written()
	for _, w := range writes[:len(writes)-1] {
		b.WriteString(w.data)
	}
	return b.String()
}

func (f typedFixture) liveDescriptor(t *testing.T) engineagents.Agent {
	t.Helper()
	d, err := f.h.rs.agents.Get(t.Context(), f.h.home, "claude")
	require.NoError(t, err)
	return d
}

func TestSubmitPrompt_AnIdleCLIWithAnEmptyComposerIsTypedInto(t *testing.T) {
	for name, setup := range map[string]func(*testing.T, typedFixture){
		"idle":                    func(*testing.T, typedFixture) {},
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
			assert.Equal(t, []termWrite{
				{sessionID: f.live.TerminalSession, data: "rename the helper"},
				{sessionID: f.live.TerminalSession, data: "\r"},
			}, f.term.written(), "the words, then the submit key as a write of its own")
			p := promptReceipt(t, f.h.rs.ledger, requestID)
			assert.Equal(t, ledger.DeliveryType, p.Delivery)
			assert.Equal(t, f.live.ID, p.RunnerID)
			assert.Equal(t, ledger.PromptStatusPending, p.Status, "the CLI's hook is its receipt")
			assert.Equal(t, 1, f.term.callCount(), "no CLI is launched")
			assert.Empty(t, f.term.terminated, "the live CLI is not displaced")
		})
	}
}

func TestSubmitPrompt_ALongMultiLineMessageIsTypedInPacedChunks(t *testing.T) {
	f := newTypedFixture(t, &typingTerm{screen: emptyComposer(t)})
	typing, ok := f.liveDescriptor(t).PromptType()
	require.True(t, ok)
	var pauses []time.Duration
	f.h.rs.pause = func(_ context.Context, d time.Duration) error { pauses = append(pauses, d); return nil }
	text := strings.Repeat("> a quoted line that goes on for a while, with words\n\nanother paragraph here.\n\n", 7)
	require.Greater(t, len(text), 500)

	_, err := f.h.rs.SubmitPrompt(t.Context(), f.h.chatID, text, uuid.NewString())

	require.NoError(t, err)
	writes := f.term.written()
	require.Greater(t, len(writes), 2)
	for _, w := range writes[:len(writes)-1] {
		assert.LessOrEqual(t, len(w.data), typing.ChunkBytes)
		assert.NotContains(t, w.data, "\x1b")
	}
	assert.Equal(t, text, f.typedText(), "every byte typed, nothing wrapped")
	assert.Equal(t, typing.Submit, writes[len(writes)-1].data)
	require.Len(t, pauses, len(writes)-1, "a pause between every pair of writes")
	for _, d := range pauses {
		assert.Equal(t, typing.ChunkInterval, d)
	}
}

func TestSubmitPrompt_ACLIThatCannotBeTypedIntoIsNotTyped(t *testing.T) {
	for name, tc := range map[string]struct {
		text     string
		setup    func(*testing.T, typedFixture)
		delivery string
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
		"an escape sequence":         {text: "a\x1b[201~b", delivery: ledger.DeliveryRestart},
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

type stuckTerm struct {
	typingTerm
	writing chan struct{}
}

func (f *stuckTerm) Write(ctx context.Context, _ string, _ []byte) error {
	close(f.writing)
	<-ctx.Done()
	return context.Cause(ctx)
}

func TestRegression_SubmitPrompt_StopDuringTypingSettlesThePromptStopped(t *testing.T) {
	term := &stuckTerm{typingTerm: typingTerm{screen: emptyComposer(t)}, writing: make(chan struct{})}
	f := newTypedFixture(t, term)
	requestID := uuid.NewString()
	submitted := make(chan error, 1)
	go func() {
		_, err := f.h.rs.SubmitPrompt(t.Context(), f.h.chatID, "hello", requestID)
		submitted <- err
	}()
	<-term.writing

	require.NoError(t, f.h.rs.StopChat(t.Context(), f.h.chatID))

	require.ErrorIs(t, <-submitted, ErrStopped)
	p := promptReceipt(t, f.h.rs.ledger, requestID)
	assert.Equal(t, ledger.PromptStatusSettled, p.Status)
	assert.Equal(t, ledger.ReasonStopped, p.Reason)
	assert.False(t, p.Consumed)
}

func TestSubmitPrompt_AResendIsNeverTypedTwice(t *testing.T) {
	f := newTypedFixture(t, &typingTerm{screen: emptyComposer(t)})
	requestID := uuid.NewString()

	_, err := f.h.rs.SubmitPrompt(t.Context(), f.h.chatID, "hello", requestID)
	require.NoError(t, err)
	_, err = f.h.rs.SubmitPrompt(t.Context(), f.h.chatID, "hello", requestID)
	require.NoError(t, err)

	assert.Equal(t, "hello", f.typedText(), "typed once, with its submit key")
	assert.Len(t, f.term.written(), 2)
}

func TestSubmitPrompt_ATypedAttachmentIsGuardedAndMaterialised(t *testing.T) {
	f := newTypedFixture(t, &typingTerm{screen: emptyComposer(t)})
	dir := worktreepath.AttachmentsDir(f.h.home, f.h.chatID)
	name, _, err := repoattachments.Store(dir, "ab12", "a.png", []byte("png"))
	require.NoError(t, err)
	stored := filepath.ToSlash(filepath.Join(dir, name))
	text := "![shot](chats/" + f.h.chatID + "/attachments/" + name + ") what is this"

	_, err = f.h.rs.SubmitPrompt(t.Context(), f.h.chatID, text, uuid.NewString())

	require.NoError(t, err)
	require.Len(t, f.term.written(), 2)
	assert.Equal(t, " ![shot]("+stored+") what is this", f.typedText())
}

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

			_, handled, err := f.h.rs.submitPromptTyped(t.Context(), t.Context(), chat, uuid.NewString(), f.live,
				testAgent(t, "claude"), "hello")

			require.NoError(t, err)
			assert.False(t, handled)
			assert.Empty(t, f.term.written())
		})
	}
}
