package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/stream"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	domainstream "github.com/char2cs/crowbar/api/internal/domain/stream"
)

type fakeWorkspaces map[string]domain.Workspace

func (f fakeWorkspaces) Get(_ context.Context, id string) (domain.Workspace, error) {
	if ws, ok := f[id]; ok {
		return ws, nil
	}
	return domain.Workspace{}, errors.New("no such workspace")
}

type fakeWalk map[string]string

func (f fakeWalk) CwdWorkspaceID(_ context.Context, chatID string) (string, bool, error) {
	id, ok := f[chatID]
	return id, ok, nil
}

// holdProjects is a client holding the projects scope, past its snapshot.
func holdProjects(t *testing.T) (*lateStream, chan domainstream.Frame) {
	t.Helper()
	encode := func(_ domainstream.EntityKind, v any) (json.RawMessage, error) { return json.Marshal(v) }
	late := &lateStream{}
	late.hub = stream.New("e1", map[domainstream.ScopeKind]stream.Source{domainstream.KindProjects: stream.Quiet{}}, nil, encode)
	conn := pipeConn{in: make(chan domainstream.Request), out: make(chan domainstream.Frame, 8)}
	go func() { _ = late.hub.Serve(t.Context(), conn) }()
	t.Cleanup(func() { close(conn.in) })
	conn.in <- domainstream.Request{Op: domainstream.OpSub, Scope: "projects"}
	for f := range conn.out {
		if f.T == domainstream.FrameSynced {
			break
		}
	}
	return late, conn.out
}

func chatFrame(id, workspaceID string, version int64) agentusecase.ChatSnapshotFrame {
	var f agentusecase.ChatSnapshotFrame
	f.Snapshot.Chat = domain.Chat{ID: id, WorkspaceID: workspaceID}
	f.Snapshot.Version = version
	return f
}

// A chat reaches the sidebar filed under its workspace's project and repo, or
// a bubble's under the workspace its cwd walk lands on; a snapshot older than
// one already sent is dropped.
func TestChatPublisher_FilesAChatUnderTheScopeItsRepoListReadsItFrom(t *testing.T) {
	late, out := holdProjects(t)
	workspaces := fakeWorkspaces{
		"w1": {ID: "w1", ProjectID: "p1", RepoID: "r1"},
		"w2": {ID: "w2", ProjectID: "p2", RepoID: "r2"},
	}
	publish := late.chatPublisher(context.Background(), workspaces, fakeWalk{"bubble": "w2"})

	type scoped struct {
		ProjectID string `json:"projectId"`
		RepoID    string `json:"repoId"`
	}
	cases := []struct {
		frame         agentusecase.ChatSnapshotFrame
		project, repo string
	}{
		{chatFrame("c1", "w1", 7), "p1", "r1"},
		{chatFrame("bubble", "", 3), "p2", "r2"},
		{chatFrame("stray", "", 1), "", ""},
	}
	for _, c := range cases {
		publish(c.frame)
		f := <-out
		require.Equal(t, domainstream.FrameEntity, f.T)
		assert.Equal(t, string(domainstream.KindChatEntity), f.Kind)
		assert.Equal(t, c.frame.Snapshot.Chat.ID, f.ID)
		var row scoped
		require.NoError(t, json.Unmarshal(f.Data, &row))
		assert.Equal(t, c.project, row.ProjectID, c.frame.Snapshot.Chat.ID)
		assert.Equal(t, c.repo, row.RepoID, c.frame.Snapshot.Chat.ID)
	}

	publish(chatFrame("c1", "w2", 6))
	publish(chatFrame("c1", "w1", 8))
	var row scoped
	require.NoError(t, json.Unmarshal((<-out).Data, &row))
	assert.Equal(t, scoped{"p1", "r1"}, row, "the stale snapshot never reached the client")
}

// What no sidebar draws any more leaves it as gone: a deleted chat, a
// tombstoned workspace, a deleted repo.
func TestSidebarPublish_ADeletedRowIsGone(t *testing.T) {
	late, out := holdProjects(t)
	deleted := chatFrame("c1", "w1", 9)
	deleted.Deleted = true

	late.chatPublisher(context.Background(), fakeWorkspaces{}, fakeWalk{})(deleted)
	late.workspace(dto.WorkspaceDTO{ID: "w1", Status: domain.WorkspaceStatusDeleted}, 4)
	(&repoEntities{late: late}).gone("r1")

	for _, want := range []struct{ kind, id string }{
		{string(domainstream.KindChatEntity), "c1"},
		{string(domainstream.KindWorkspace), "w1"},
		{string(domainstream.KindRepo), "r1"},
	} {
		f := <-out
		assert.True(t, f.Gone, want.id)
		assert.Equal(t, want.kind, f.Kind)
		assert.Equal(t, want.id, f.ID)
	}
}

// A workspace row read before one already sent is dropped, and no row read
// before its tombstone brings it back.
func TestSidebarPublish_AStaleWorkspaceRowNeverOverwrites(t *testing.T) {
	late, out := holdProjects(t)

	late.workspace(dto.WorkspaceDTO{ID: "w1", FolderID: "f2"}, 5)
	late.workspace(dto.WorkspaceDTO{ID: "w1", FolderID: "f1"}, 4)
	late.workspace(dto.WorkspaceDTO{ID: "w1", Status: domain.WorkspaceStatusDeleted}, 6)
	late.workspace(dto.WorkspaceDTO{ID: "w1", FolderID: "f2"}, 6)
	late.workspace(dto.WorkspaceDTO{ID: "w2"}, 1)

	var got []string
	for range 3 {
		f := <-out
		var row dto.WorkspaceDTO
		if !f.Gone {
			require.NoError(t, json.Unmarshal(f.Data, &row))
		}
		got = append(got, fmt.Sprintf("%s:%s:%t", f.ID, row.FolderID, f.Gone))
	}
	assert.Equal(t, []string{"w1:f2:false", "w1::true", "w2::false"}, got)
}
