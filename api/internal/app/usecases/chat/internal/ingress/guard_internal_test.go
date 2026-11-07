package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func movedOutOf(t *testing.T, from, to string) ledger.State {
	t.Helper()
	r, err := ledger.NewRow(ledger.KindMove, "", "", ledger.Move{
		Phase: ledger.MoveDone, RequestID: "m1", To: "ws-b", FromDir: from, ToDir: to,
	})
	require.NoError(t, err)
	r.ChatID, r.Seq = "c1", 1
	return ledger.Fold("c1", []ledger.Row{r})
}

func toolPre(kind, target, cwd string) engineagents.CanonicalEvent {
	return engineagents.CanonicalEvent{
		Kind: engineagents.HookToolPre,
		Tool: &engineagents.ToolEvent{Kind: kind, Target: target, Cwd: cwd},
	}
}

func TestLeftFolderWrite_RefusesAnEditInTheFolderTheChatLeft(t *testing.T) {
	st := movedOutOf(t, "/home/p", "/wt/b2")
	assert.Equal(t, "/home/p", leftFolderWrite(st, toolPre("edit", "/home/p/note.txt", "/wt/b2")))
	assert.Equal(t, "/home/p", leftFolderWrite(st, toolPre("edit", "../../home/p/x", "/wt/b2")))
	assert.Empty(t, leftFolderWrite(st, toolPre("edit", "/wt/b2/note.txt", "/wt/b2")))
	assert.Empty(t, leftFolderWrite(st, toolPre("edit", "/home/pp/x", "/wt/b2")), "a sibling folder is not the one left")
	assert.Empty(t, leftFolderWrite(st, toolPre("read", "/home/p/note.txt", "/wt/b2")), "reading it is allowed")
}

func TestLeftFolderWrite_ACommandNamingTheFolderIsRefused(t *testing.T) {
	st := movedOutOf(t, "/home/p", "/wt/b2")
	assert.Equal(t, "/home/p", leftFolderWrite(st, toolPre("execute", `echo hi > "/home/p/x"`, "")))
	assert.Empty(t, leftFolderWrite(st, toolPre("execute", "echo hi > x", "")))
}

func TestLeftFolderWrite_AChatThatNeverMovedIsNotGuarded(t *testing.T) {
	assert.Empty(t, leftFolderWrite(ledger.State{}, toolPre("edit", "/anywhere/x", "")))
}
