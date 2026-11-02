//go:build integration

package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

func isReady(f stream.Frame) bool { return f.T == stream.FrameReady }

func isSynced(scope string) func(stream.Frame) bool {
	return func(f stream.Frame) bool { return f.T == stream.FrameSynced && f.Scope == scope }
}

func TestStream_FirstFrameIsReady(t *testing.T) {
	h := newHarness(t)
	c := dialStream(t, h)

	first := c.Next(t, func(stream.Frame) bool { return true })

	assert.Equal(t, stream.FrameReady, first.T)
	assert.NotEmpty(t, first.Epoch)
}

// A new daemon's versions start over, so the client must be told to drop what it holds.
func TestStream_ReadyCarriesANewEpochAfterRestart(t *testing.T) {
	h := newHarness(t)
	before := dialStream(t, h).Next(t, isReady).Epoch
	h.shutdown()

	again := newHarnessAt(t, h.home)
	after := dialStream(t, again).Next(t, isReady).Epoch

	assert.NotEmpty(t, after)
	assert.NotEqual(t, before, after)
}

func TestStream_ProjectsScopeSnapshotsOperations(t *testing.T) {
	h := newHarness(t)
	push := failingPush(t, h)
	op := startOperation(t, h, "/v0/chats/"+push.chatID+"/git/push", nil)
	awaitOperation(t, h, op.ID, operationSettled)

	c := dialStream(t, h)
	c.Sub(t, "projects", 0)
	var snapshot *domain.Operation
	c.Next(t, func(f stream.Frame) bool {
		if isOperationFrame(f, op.ID) {
			got := operationIn(t, f)
			snapshot = &got
		}
		return isSynced("projects")(f)
	})

	require.NotNil(t, snapshot, "the settled operation arrives before synced")
	assert.Equal(t, domain.OpFailed, snapshot.State)
}

func TestStream_AnOperationFrameFollowsItsStateChanges(t *testing.T) {
	h := newHarness(t)
	push := failingPush(t, h)
	c := dialStream(t, h)
	c.Sub(t, "projects", 0)
	c.Next(t, isSynced("projects"))

	op := startOperation(t, h, "/v0/chats/"+push.chatID+"/git/push", nil)
	var states []domain.OperationState
	var last uint64
	c.Next(t, func(f stream.Frame) bool {
		if !isOperationFrame(f, op.ID) {
			return false
		}
		assert.Greater(t, f.V, last, "each frame of the operation carries a newer version")
		last = f.V
		got := operationIn(t, f)
		states = append(states, got.State)
		return operationSettled(got)
	})

	assert.Equal(t, []domain.OperationState{domain.OpAccepted, domain.OpRunning, domain.OpFailed}, states)
}

func TestStream_AnUnknownScopeIsRefusedWithAProblem(t *testing.T) {
	h := newHarness(t)
	c := dialStream(t, h)
	c.Sub(t, "nope:x", 0)

	closed := c.Next(t, func(f stream.Frame) bool { return f.T == stream.FrameClosed })

	assert.Equal(t, "nope:x", closed.Scope)
	assert.Equal(t, stream.ReasonRefused, closed.Reason)
	require.NotNil(t, closed.Problem)
	assert.Equal(t, problem.InvalidArgument, closed.Problem.Code)

	c.Sub(t, "projects", 0)
	c.Next(t, isSynced("projects")) // the connection still serves
}

// Review Focus 2: a resume sends every row after since once, in order, then synced at the head.
func TestStream_ChatScopeResumesFromSinceWithoutGapOrDuplicate(t *testing.T) {
	h := newHarness(t)
	_, chatID, post := newFeedChat(t, h, "streamstub", streamStubProviderDescriptorYAML)
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"hello"}`)
	scope := "chat:" + chatID
	c := dialStream(t, h)
	c.Next(t, isReady)
	c.Sub(t, scope, 0)
	head := c.Next(t, isSynced(scope)).Seq
	require.GreaterOrEqual(t, head, int64(2))
	// The resume is a reconnect: frames the first connection still had in
	// flight cannot reach the second.
	require.NoError(t, c.conn.Close())

	for i := range 3 {
		post("message_delta", delta(fmt.Sprintf("msg-%d", i), 0, true, "HI"))
	}
	c = dialStream(t, h)
	c.Next(t, isReady)
	c.Sub(t, scope, head-1)
	var seqs []int64
	synced := c.Next(t, func(f stream.Frame) bool {
		if f.Scope == scope && f.T == stream.FrameRow {
			seqs = append(seqs, f.Row.Seq)
		}
		return isSynced(scope)(f)
	})

	assert.Equal(t, contiguous(head, head+3), seqs)
	assert.Equal(t, head+3, synced.Seq)
	post("message_delta", delta("msg-live", 0, true, "HI"))
	live := c.Next(t, func(f stream.Frame) bool { return f.Scope == scope && f.T == stream.FrameRow })
	assert.Equal(t, head+4, live.Row.Seq, "live rows follow the replay with no gap")
}

// A pane that lets go of a chat and takes it back on one connection may still
// get a row the first sub had in flight. A client that applies only the row
// after its last one, as the web does, ends with every row once and no gap.
func TestStream_ChatScopeResubscribedOnOneConnectionHasNoGap(t *testing.T) {
	h := newHarness(t)
	_, chatID, post := newFeedChat(t, h, "streamstub", streamStubProviderDescriptorYAML)
	post("session_start", `{"session_id":"sess-1"}`)
	post("user_prompt", `{"session_id":"sess-1","prompt":"hello"}`)
	scope := "chat:" + chatID
	c := dialStream(t, h)
	c.Next(t, isReady)
	c.Sub(t, scope, 0)
	head := c.Next(t, isSynced(scope)).Seq

	c.Unsub(t, scope)
	for i := range 3 {
		post("message_delta", delta(fmt.Sprintf("msg-%d", i), 0, true, "HI"))
	}
	c.Sub(t, scope, head)
	applied, rows := head, []int64{}
	synced := c.Next(t, func(f stream.Frame) bool {
		if f.Scope == scope && f.T == stream.FrameRow && f.Row.Seq == applied+1 {
			applied++
			rows = append(rows, applied)
		}
		return isSynced(scope)(f)
	})

	assert.Equal(t, contiguous(head+1, head+3), rows)
	assert.Equal(t, applied, synced.Seq, "the synced names the last row applied")
}

func TestStream_DeletedChatClosesItsScopeOnly(t *testing.T) {
	h := newHarness(t)
	imported, chatID, _ := newFeedChat(t, h, "streamstub", streamStubProviderDescriptorYAML)
	push := failingPush(t, h)
	scope := "chat:" + chatID
	c := dialStream(t, h)
	c.Hold(t, "projects")
	c.Hold(t, scope)

	_ = h.raw(http.MethodDelete, repoBase(imported)+"/chats/"+chatID, nil, http.StatusOK).Body.Close()
	closed := c.Next(t, func(f stream.Frame) bool { return f.Scope == scope && f.T == stream.FrameClosed })
	assert.Equal(t, stream.Closed(scope, stream.ReasonDeleted, nil), closed)

	op := startOperation(t, h, "/v0/chats/"+push.chatID+"/git/push", nil)
	c.Next(t, func(f stream.Frame) bool { return isOperationFrame(f, op.ID) })
}

// sidebarEntity is one projects-scope entity as a test reads it.
type sidebarEntity struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	RepoID    string `json:"repoId"`
	ParentID  string `json:"parentId"`
	Name      string `json:"name"`
}

func entityIn(t *testing.T, f stream.Frame) sidebarEntity {
	t.Helper()
	var e sidebarEntity
	require.NoError(t, json.Unmarshal(f.Data, &e))
	return e
}

// sidebarSnapshot holds projects and answers its snapshot, keyed kind/id.
func sidebarSnapshot(t *testing.T, c *streamClient) map[string]sidebarEntity {
	t.Helper()
	c.Sub(t, "projects", 0)
	got := map[string]sidebarEntity{}
	c.Next(t, func(f stream.Frame) bool {
		if f.T == stream.FrameEntity && f.Kind != string(stream.KindOperation) {
			got[f.Kind+"/"+f.ID] = entityIn(t, f)
		}
		return isSynced("projects")(f)
	})
	return got
}

func TestStream_ProjectsScopeCarriesTheSidebar(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	writeLiveStubProviderDescriptor(t, h)
	folder := createChatFolder(t, h, repoBase(imported), "Docs", "")
	homeChat := createHomeChat(t, h, imported.projectID)

	snapshot := sidebarSnapshot(t, dialStream(t, h))

	assert.Contains(t, snapshot, "project/"+imported.projectID)
	assert.Equal(t, imported.projectID, snapshot["repo/"+imported.repoID].ProjectID)
	assert.Equal(t, imported.repoID, snapshot["workspace/"+imported.workspaceID].RepoID)
	assert.Equal(t, sidebarEntity{ID: imported.chatID, ProjectID: imported.projectID, RepoID: imported.repoID},
		snapshot["chat/"+imported.chatID])
	assert.Equal(t, sidebarEntity{ID: folder.ID, ProjectID: imported.projectID, RepoID: imported.repoID, Name: "Docs"},
		snapshot["folder/"+folder.ID])
	assert.Equal(t, sidebarEntity{ID: homeChat, ProjectID: imported.projectID}, snapshot["chat/"+homeChat],
		"a home chat is listed under its project and no repo")
}

// A live change reaches the holder as the entity the snapshot would carry.
func TestStream_ProjectsScopeFollowsALiveFolder(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	c := dialStream(t, h)
	c.Hold(t, "projects")

	folder := createChatFolder(t, h, repoBase(imported), "Live", "")
	got := c.Next(t, func(f stream.Frame) bool {
		return f.Kind == string(stream.KindFolder) && f.ID == folder.ID
	})

	assert.Equal(t, sidebarEntity{ID: folder.ID, ProjectID: imported.projectID, RepoID: imported.repoID, Name: "Live"},
		entityIn(t, got))
}

// Index R16: an operation's succeeded frame comes after every entity its work wrote.
func TestStream_OperationSucceededFollowsItsEntities(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)
	f.pushFileToOrigin(t, "feature/a", "main", "a.txt", "a\n")
	f.pushFileToOrigin(t, "feature/b", "main", "b.txt", "b\n")
	c := dialStream(t, h)
	c.Hold(t, "projects")

	heard := map[string]bool{}
	await := func(op domain.Operation, also func(stream.Frame)) domain.Operation {
		settled := c.Next(t, func(fr stream.Frame) bool {
			heard[fr.Kind+"/"+fr.ID] = true
			if also != nil && fr.T == stream.FrameEntity && !fr.Gone && fr.Kind == string(stream.KindWorkspace) {
				also(fr)
			}
			return isOperationFrame(fr, op.ID) && operationSettled(operationIn(t, fr))
		})
		return operationIn(t, settled)
	}
	imported := await(startOperation(t, h, f.repoBase()+"/chats/import-batch", importBranches("feature/a", "feature/b")), nil)
	require.Equal(t, domain.OpSucceeded, imported.State)
	h.Quiesce()
	rows := map[string]chatRow{}
	for _, row := range listChats(t, h, f.projectID, f.repoID) {
		if row.Worktree != nil && row.WorkspaceID != "" {
			rows[row.Worktree.Branch] = row
		}
	}
	for _, b := range []string{"feature/a", "feature/b"} {
		require.Contains(t, rows, b)
		assert.True(t, heard[string(stream.KindWorkspace)+"/"+rows[b].WorkspaceID], "%s's workspace preceded the import's success", b)
		assert.True(t, heard[string(stream.KindChatEntity)+"/"+owningChatID(t, h, rows[b].WorkspaceID)], "%s's chat preceded the import's success", b)
	}

	child, parent := rows["feature/b"].WorkspaceID, rows["feature/a"].WorkspaceID
	var moved bool
	reparented := await(startOperation(t, h, f.repoBase()+"/chats/"+owningChatID(t, h, child)+"/reparent",
		map[string]any{"newParentId": parent}), func(fr stream.Frame) {
		moved = moved || fr.ID == child && entityIn(t, fr).ParentID == parent
	})
	require.Equal(t, domain.OpSucceeded, reparented.State)
	assert.True(t, moved, "the moved workspace reached the client before its operation settled")
}

func TestStream_ADeletedChatIsGoneOnTheProjectsScope(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	writeLiveStubProviderDescriptor(t, h)
	chatID := createAgentChat(t, h, imported)
	c := dialStream(t, h)
	c.Hold(t, "projects")

	_ = h.raw(http.MethodDelete, repoBase(imported)+"/chats/"+chatID, nil, http.StatusOK).Body.Close()
	gone := c.Next(t, func(f stream.Frame) bool {
		return f.Kind == string(stream.KindChatEntity) && f.ID == chatID && f.Gone
	})

	assert.True(t, gone.Gone)
	assert.NotContains(t, sidebarSnapshot(t, dialStream(t, h)), "chat/"+chatID)
}

// A live chat is filed under the scope its workspace answers: a repo chat in
// any of its repo's workspaces under that repo, a home chat under its project
// and no repo, whichever project or repo wrote last.
func TestStream_ALiveChatIsFiledUnderItsOwnScope(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	a := importProject(t, h)
	b := importProject(t, h)
	otherWS := createChildWorkspace(t, h, a, "feature/other", a.workspaceID)
	h.Quiesce()
	feed := recordSidebar(t, h)

	var inChild, inB struct {
		ID string `json:"id"`
	}
	h.post(repoBase(a)+"/chats", map[string]string{"provider": "livestub", "workspaceId": otherWS},
		http.StatusCreated, &inChild)
	h.post(repoBase(b)+"/chats", map[string]string{"provider": "livestub", "workspaceId": b.workspaceID},
		http.StatusCreated, &inB)
	home := createHomeChat(t, h, b.projectID)

	assert.Equal(t, []any{a.projectID, a.repoID}, scopeOf(feed.awaitListed(inChild.ID)))
	assert.Equal(t, []any{b.projectID, b.repoID}, scopeOf(feed.awaitListed(inB.ID)))
	assert.Equal(t, []any{b.projectID, ""}, scopeOf(feed.awaitListed(home)))
}

func scopeOf(chat map[string]any) []any { return []any{chat["projectId"], chat["repoId"]} }

// A tab open while a repo imports, a branch chat forks and an owning chat is
// deleted ends holding each workspace's owner as a fresh tab reads it.
func TestStream_AnOpenTabHoldsEachWorkspacesOwner(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	feed := recordSidebar(t, h)
	imported := importProject(t, h)
	forkFromSidebar(t, h, imported, imported.chatID, "feat/owner")
	forked, _ := forkFromSidebar(t, h, imported, imported.chatID, "feat/orphan")
	_ = h.raw(http.MethodDelete, repoBase(imported)+"/chats/"+forked, nil, http.StatusOK).Body.Close()
	h.QuiesceReactors()
	h.Quiesce()

	fresh := recordSidebar(t, h)
	held, _ := fresh.snapshot()
	owned := 0
	for _, r := range held {
		if r.Kind != string(stream.KindWorkspace) {
			continue
		}
		owner := r.Data["owningChatId"]
		if owner != "" {
			owned++
		}
		feed.await(fmt.Sprintf("workspace %s owned by %q", r.ID, owner), func(i int, live feedRow) bool {
			return live.Kind == r.Kind && live.ID == r.ID && i == lastRowOf(feed, r.Kind, r.ID) &&
				!live.Gone && live.Data["owningChatId"] == owner
		})
	}
	require.Positive(t, owned, "the fresh tab holds owned workspaces")
}

// lastRowOf is the index of the newest row f recorded for kind/id.
func lastRowOf(f *sidebarFeed, kind, id string) int {
	last := -1
	for i, r := range f.rows() {
		if r.Kind == kind && r.ID == id {
			last = i
		}
	}
	return last
}
