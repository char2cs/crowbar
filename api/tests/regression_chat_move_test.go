//go:build integration

package tests

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/tests/kit"
)

// moveWorld is one chat on the fake claude, bound to session after a whole turn, in the
// writable workspace, and a second branch (wsB, owned by chatB) to move it into.
type moveWorld struct {
	h                  *harness
	imported           importedRepo
	fake               kit.FakeProvider
	chatID, session    string
	wsB, chatB, branch string
	hookBase           string
}

func newMoveWorld(t *testing.T, h *harness) moveWorld {
	t.Helper()
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	chatID, l := createFakeChat(t, h, imported, fake)
	w := moveWorld{
		h: h, imported: imported, fake: fake, chatID: chatID, session: argAfter(l.Argv, "--session-id"),
		branch: "feature/b2", hookBase: h.url + repoBase(imported),
	}
	require.NotEmpty(t, w.session, "the first launch names its session: %v", l.Argv)
	completeTurn(t, h, l, w.hookBase, "claude", w.session, "one")
	w.wsB, w.chatB = createWorktree(t, h, imported, w.branch, imported.workspaceID)
	return w
}

// moveInto drops chatID under parentID in workspaceID, as the sidebar's drag does.
func (w moveWorld) moveInto(t *testing.T, chatID, parentID, workspaceID string) {
	t.Helper()
	w.h.patch(repoBase(w.imported)+"/chats/"+chatID+"/placement",
		map[string]any{"parentId": parentID, "workspaceId": workspaceID, "label": w.branch}, nil)
	w.h.QuiesceReactors()
}

// openTurn launches the chat on a prompt and opens its turn, as the CLI reports it.
func (w moveWorld) openTurn(t *testing.T) kit.Launch {
	t.Helper()
	l := promptLaunch(t, w.h, w.imported, w.fake, w.chatID, "work", "")
	postSessionHook(t, l, w.hookBase, "claude", "session_start", map[string]any{"session_id": w.session, "source": "resume"})
	postSessionHook(t, l, w.hookBase, "claude", "user_prompt", map[string]any{"session_id": w.session, "prompt": "work"})
	w.h.QuiesceReactors()
	require.True(t, chatBusy(t, w.h, w.chatID), "precondition: the chat is mid-turn")
	return l
}

func (w moveWorld) closeTurn(t *testing.T, l kit.Launch) {
	t.Helper()
	postSessionHook(t, l, w.hookBase, "claude", "turn_stop", map[string]any{"session_id": w.session, "last_assistant_message": "ok"})
	w.h.QuiesceReactors()
}

func (w moveWorld) workspaceOf(t *testing.T, chatID string) string {
	t.Helper()
	return getAgentChat(t, w.h, repoBase(w.imported), chatID).WorkspaceID
}

func (w moveWorld) worktree(t *testing.T, workspaceID string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(worktreeOf(t, w.h, w.imported, workspaceID).LocalPath)
	require.NoError(t, err)
	return dir
}

func moveRows(t *testing.T, h *harness, chatID string) []ledger.Move {
	t.Helper()
	return kit.Bodies[ledger.Move](ledgerRows(t, h, chatID), ledger.KindMove)
}

// untilMoved waits for chatID's newest move row to be done: its row is written after the
// chat runs in its new place.
func untilMoved(t *testing.T, h *harness, chatID string) {
	t.Helper()
	require.Eventually(t, func() bool {
		moves := moveRows(t, h, chatID)
		return len(moves) > 0 && moves[len(moves)-1].Phase == ledger.MoveDone
	}, 10*time.Second, 10*time.Millisecond, "chat %s's move was never done: %+v", chatID, moveRows(t, h, chatID))
}

// A dormant thread dropped into another branch moves at once with the thread under it, and its
// next prompt resumes the same session in the new worktree.
func TestRegression_Move_ADroppedThreadContinuesItsSessionInTheNewWorktree(t *testing.T) {
	w := newMoveWorld(t, newHarness(t))
	child := createChatWithProvider(t, w.h, repoBase(w.imported), w.fake.ID, w.imported.workspaceID, w.chatID)

	w.moveInto(t, w.chatID, w.chatB, w.wsB)

	untilMoved(t, w.h, w.chatID)
	untilMoved(t, w.h, child)
	assert.Equal(t, w.wsB, w.workspaceOf(t, w.chatID))
	assert.Equal(t, w.wsB, w.workspaceOf(t, child), "the thread under it follows")
	l := promptLaunch(t, w.h, w.imported, w.fake, w.chatID, "where now?", "")
	assert.Equal(t, w.worktree(t, w.wsB), l.Cwd, "the CLI starts in the worktree it moved to")
	assert.Equal(t, w.session, argAfter(l.Argv, "--resume"), "the same conversation, resumed by its id: %v", l.Argv)
}

// A working chat keeps running where it is; its move waits for the turn to end.
func TestRegression_Move_AWorkingChatMovesAtItsTurnBoundary(t *testing.T) {
	w := newMoveWorld(t, newHarness(t))
	l := w.openTurn(t)

	w.moveInto(t, w.chatID, w.chatB, w.wsB)
	assert.Equal(t, w.imported.workspaceID, w.workspaceOf(t, w.chatID), "never moved mid-turn")
	pending, ok := chatState(t, w.h, w.chatID).PendingMove()
	require.True(t, ok, "the move waits on the chat, for the sidebar to show")
	assert.Equal(t, w.wsB, pending.To)

	w.closeTurn(t, l)
	untilMoved(t, w.h, w.chatID)
	assert.Equal(t, w.wsB, w.workspaceOf(t, w.chatID))
}

// Cancel move keeps the chat where it runs.
func TestRegression_Move_ACancelledMoveLeavesTheChatWhereItRuns(t *testing.T) {
	w := newMoveWorld(t, newHarness(t))
	l := w.openTurn(t)
	w.moveInto(t, w.chatID, w.chatB, w.wsB)

	w.h.post(repoBase(w.imported)+"/chats/"+w.chatID+"/move/cancel", nil, http.StatusOK, nil)
	w.closeTurn(t, l)

	_, pending := chatState(t, w.h, w.chatID).PendingMove()
	assert.False(t, pending)
	assert.Equal(t, w.imported.workspaceID, w.workspaceOf(t, w.chatID))
	moves := moveRows(t, w.h, w.chatID)
	assert.Equal(t, ledger.MoveCancelled, moves[len(moves)-1].Phase)
}

// A move asked for before the daemon died is made by the next boot, not lost or failed.
func TestRegression_Move_APendingMoveSurvivesARestart(t *testing.T) {
	for _, stop := range []string{"shutdown", "crash"} {
		t.Run(stop, func(t *testing.T) {
			home := t.TempDir()
			w := newMoveWorld(t, newHarnessAt(t, home))
			w.openTurn(t)
			w.moveInto(t, w.chatID, w.chatB, w.wsB)
			if stop == "crash" {
				w.h.crash()
			} else {
				w.h.shutdown()
			}

			w.h = newHarnessAt(t, home)
			w.h.Quiesce()
			untilMoved(t, w.h, w.chatID)
			assert.Equal(t, w.wsB, w.workspaceOf(t, w.chatID))
		})
	}
}

// Make a branch cuts from the folder's own last commit, not origin's: a local commit comes
// along, an untracked file does not, and the thread then owns the new branch.
func TestRegression_BranchOut_CutsFromTheLocalHeadAndTheThreadOwnsIt(t *testing.T) {
	w := newMoveWorld(t, newHarness(t))
	here := w.worktree(t, w.imported.workspaceID)
	require.NoError(t, writeFile(here, "local.txt", "committed here only\n"))
	runGit(t, here, "add", "local.txt")
	runGit(t, here, "commit", "-m", "local only")
	require.NoError(t, writeFile(here, "scratch.txt", "never committed\n"))

	w.h.post(repoBase(w.imported)+"/chats/"+w.chatID+"/branch-out",
		map[string]string{"branch": "cut-" + strings.Split(uuid.NewString(), "-")[0]}, http.StatusAccepted, nil)
	w.h.QuiesceReactors()

	require.Eventually(t, func() bool {
		ws := w.workspaceOf(t, w.chatID)
		return ws != "" && ws != w.imported.workspaceID
	}, 10*time.Second, 10*time.Millisecond, "the thread never ran in its own branch")
	cut := w.worktree(t, w.workspaceOf(t, w.chatID))
	_, err := os.Stat(filepath.Join(cut, "local.txt"))
	require.NoError(t, err, "the branch starts at the folder's local commit")
	_, err = os.Stat(filepath.Join(cut, "scratch.txt"))
	assert.True(t, os.IsNotExist(err), "uncommitted files stay behind")
	assert.Equal(t, w.chatID, worktreeOf(t, w.h, w.imported, w.workspaceOf(t, w.chatID)).OwningChatID)
}

// A drop naming a workspace other than the one its container runs in is refused, and nothing
// is asked for: the thread would run somewhere its row does not say.
func TestRegression_Move_AContainerOfAnotherWorkspaceIsRefused(t *testing.T) {
	w := newMoveWorld(t, newHarness(t))
	wsC, _ := createWorktree(t, w.h, w.imported, "feature/c", w.imported.workspaceID)

	msg := w.h.mutationError(http.MethodPatch, repoBase(w.imported)+"/chats/"+w.chatID+"/placement",
		map[string]any{"parentId": w.chatB, "workspaceId": wsC}, http.StatusConflict)

	assert.Contains(t, msg, "stays in its chat's workspace")
	_, pending := chatState(t, w.h, w.chatID).PendingMove()
	assert.False(t, pending)
}
