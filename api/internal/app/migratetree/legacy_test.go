package migratetree

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

// legacy is a home as the retired build left it: the chat log at schema 1, the
// node log, the folders table, and the facts that place them.
type legacy struct {
	h          home.Home
	chatLog    asynxModels.Store
	view       *gormdb.DB
	projects   []domain.Project
	workspaces []domain.Workspace
	repos      []domain.Repository
}

func newLegacy(t *testing.T) *legacy {
	t.Helper()
	h := testutil.FixtureHome(t)
	require.NoError(t, os.MkdirAll(h.Events(), 0o700))
	chatLog, err := eventsqlite.NewEventStore(filepath.Join(h.Events(), "agent_chat.db"))
	require.NoError(t, err)
	view, err := storesqlite.OpenDB(filepath.Join(h.State(), "view.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		closeLog(context.Background(), chatLog)
		if db, err := view.DB(); err == nil {
			require.NoError(t, db.Close())
		}
	})
	return &legacy{h: h, chatLog: chatLog, view: view}
}

func (l *legacy) nodeLog() string { return filepath.Join(l.h.Events(), "node.db") }

func (l *legacy) sources() LegacySources {
	return LegacySources{
		ChatLog: l.chatLog, NodeLog: l.nodeLog(), View: l.view,
		Projects:   func(context.Context) ([]domain.Project, error) { return l.projects, nil },
		Workspaces: func(context.Context) ([]domain.Workspace, error) { return l.workspaces, nil },
		Repos:      func(context.Context) ([]domain.Repository, error) { return l.repos, nil },
	}
}

func (l *legacy) project(id, homeID string, made time.Time) {
	l.projects = append(l.projects, domain.Project{ID: id})
	l.workspaces = append(l.workspaces,
		domain.Workspace{ID: homeID, ProjectID: id, Kind: domain.WorkspaceKindHome, CreatedAt: made})
}

func (l *legacy) workspace(id, projectID, repoID string) {
	l.workspaces = append(l.workspaces, domain.Workspace{ID: id, ProjectID: projectID, RepoID: repoID})
}

func (l *legacy) repo(id, projectID string) {
	l.repos = append(l.repos, domain.Repository{ID: id, ProjectID: projectID})
}

// write is a schema-1 write of a whole document.
type write[T any] struct {
	id  string
	doc T
}

func (w write[T]) AggregateID() string  { return w.id }
func (w write[T]) EventName() string    { return "legacy.written" }
func (w write[T]) ShouldSnapshot() bool { return false }
func (w write[T]) Validate(*T) error    { return nil }
func (w write[T]) EmitEvent(*T) T       { return w.doc }
func writeAll[T any](t *testing.T, log asynxModels.Store, docs map[string][]T) {
	t.Helper()
	ctx := context.Background()
	ax, err := asynx.New[T]().WithEventStore(log).WithSnapshotStore(asynxstore.NewSnapshots()).
		WithSchemaVersion(1).Build()
	require.NoError(t, err)
	defer func() { require.NoError(t, ax.Shutdown(ctx)) }()
	for id, history := range docs {
		for _, doc := range history {
			_, err := ax.SendWait(ctx, write[T]{id: id, doc: doc})
			require.NoError(t, err)
		}
	}
}

// chat writes c's history: its creation at the root, then its placement, so
// the log holds real /parentId and /order patches.
func (l *legacy) chat(t *testing.T, id, workspaceID, parentID string, order int) {
	t.Helper()
	born := legacyChat{Chat: domain.Chat{ID: id, WorkspaceID: workspaceID, Type: domain.ChatTypeChat}}
	placed := born
	placed.ParentID, placed.Order = parentID, order
	writeAll(t, l.chatLog, map[string][]legacyChat{id: {born, placed}})
}

func (l *legacy) nodes(t *testing.T, nodes ...legacyNode) {
	t.Helper()
	log, err := eventsqlite.NewEventStore(l.nodeLog())
	require.NoError(t, err)
	defer closeLog(context.Background(), log)
	docs := map[string][]legacyNode{}
	for _, n := range nodes {
		docs[n.ID] = append(docs[n.ID], n)
	}
	writeAll(t, log, docs)
}

func (l *legacy) folder(t *testing.T, f tree.Folder) {
	t.Helper()
	require.NoError(t, l.view.Exec(`CREATE TABLE IF NOT EXISTS folders
		(id text PRIMARY KEY, name text, repo_id text, home_id text)`).Error)
	require.NoError(t, l.view.Exec("INSERT INTO folders (id, name, repo_id, home_id) VALUES (?, ?, ?, ?)",
		f.ID, f.Name, f.RepoID, f.HomeID).Error)
}

func TestReadLegacy_ReadsEveryRetiredRecordAsWritten(t *testing.T) {
	l := newLegacy(t)
	l.chat(t, "c-1", "ws-1", "f-1", 3)
	l.nodes(t, legacyNode{ID: "f-1", Kind: "folder", Order: 2}, legacyNode{ID: "c-2", Kind: "chat", ParentID: "f-1"})
	l.folder(t, tree.Folder{ID: "f-1", Name: "Ideas", HomeID: "h-1"})

	got, err := ReadLegacy(context.Background(), l.sources())

	require.NoError(t, err)
	assert.Equal(t, []LegacyChat{{ID: "c-1", WorkspaceID: "ws-1", Placement: tree.Placement{ParentID: "f-1", Order: 3}}},
		got.Chats, "a schema-1 history still carries the placement it patched")
	assert.ElementsMatch(t, []tree.Node{
		{ID: "f-1", Kind: tree.KindFolder, Order: 2},
		{ID: "c-2", Kind: tree.KindChat, ParentID: "f-1"},
	}, got.Nodes)
	assert.Equal(t, []tree.Folder{{ID: "f-1", Name: "Ideas", HomeID: "h-1"}}, got.Folders)
}

func TestReadLegacy_AFreshHomeHasNoRetiredRecords(t *testing.T) {
	l := newLegacy(t)

	got, err := ReadLegacy(context.Background(), l.sources())

	require.NoError(t, err)
	assert.Empty(t, got.Nodes)
	assert.Empty(t, got.Folders)
	assert.Empty(t, got.Chats)
	assert.NoFileExists(t, l.nodeLog(), "a read creates no node log")
}
