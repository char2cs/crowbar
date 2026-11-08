package ingress

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

const testWorktree = "/ws/repo"

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

// codex's file change names no directory: the folder its process runs in is the
// one its paths are named from, as a payload's own cwd is.
func TestRecorder_ACallWhosePayloadNamesNoDirectoryIsNamedFromTheRunners(t *testing.T) {
	f := newRecorderFixture(t, "codex")
	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"})
	require.NoError(t, err)

	_, _, err = f.dispatch(t, toolPre("edit", testWorktree+"/src/a.go", ""))
	require.NoError(t, err)

	var targets []string
	for _, r := range f.rows(t) {
		if call, err := ledger.DecodeBody[ledger.ToolCall](r); r.Kind == ledger.KindToolCall && err == nil {
			targets = append(targets, call.Target)
		}
	}
	assert.Equal(t, []string{"src/a.go"}, targets)
}

// codexApproval is a recorded codex approval request as its api driver hands it on: its
// params, named by the wire method's tool.
func codexApproval(t *testing.T, f *recorderFixture, name, tool string) (engineagents.CanonicalEvent, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "engine", "agents", "internal", "protocol",
		"testdata", "fixtures", "codex", name))
	require.NoError(t, err)
	var frame struct{ Params map[string]any }
	require.NoError(t, json.Unmarshal(data, &frame))
	frame.Params["tool"] = tool
	raw, err := json.Marshal(frame.Params)
	require.NoError(t, err)
	ev, err := f.agent.ParseHook(engineagents.HookPermission, raw, engineagents.ChannelAPI)
	require.NoError(t, err)
	return ev, raw
}

// An agent whose sandbox asks before writing into the folder its chat left is refused
// there and then: no card asks a person to allow what Crowbar would refuse.
func TestRecorder_AnAskToWriteIntoAFolderTheChatLeftIsRefusedUnasked(t *testing.T) {
	f := newRecorderFixture(t, "codex")
	moved, err := ledger.NewRow(ledger.KindMove, "", "", ledger.Move{
		Phase: ledger.MoveDone, RequestID: "m1", To: "ws-b", FromDir: "/old", ToDir: testWorktree,
	})
	require.NoError(t, err)
	_, _, err = f.ledger.Append(t.Context(), testChat, moved)
	require.NoError(t, err)
	_, _, err = f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"})
	require.NoError(t, err)
	_, _, err = f.r.Dispatch(t.Context(), f.runner, f.agent, engineagents.CanonicalEvent{
		Kind: engineagents.HookToolPre,
		Tool: &engineagents.ToolEvent{ID: "c1", Name: "fileChange", Kind: "edit", Target: "/old/notes.txt"},
	}, nil, engineagents.ChannelAPI)
	require.NoError(t, err)
	ev, raw := codexApproval(t, f, "item_fileChange_requestApproval.json", "fileChange")

	reply, ask, err := f.r.Dispatch(t.Context(), f.runner, f.agent, ev, raw, engineagents.ChannelAPI)

	require.NoError(t, err)
	assert.JSONEq(t, `{"decision":"decline"}`, string(reply))
	assert.Empty(t, ask)
	assert.Empty(t, f.state(t).PendingAsks(), "no card is raised")
}
