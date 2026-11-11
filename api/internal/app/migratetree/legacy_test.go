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

// branch is a workspace of the fixture's repo, in that repo's project.
func (l *legacy) branch(id string) {
	r := l.repos[0]
	l.workspaces = append(l.workspaces, domain.Workspace{ID: id, ProjectID: r.ProjectID, RepoID: r.ID})
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

// schema1Chat is the whole chat document schema 1 wrote: its placement and
// the turn state the ledger now folds.
type schema1Chat struct {
	legacyChat
	Working            bool       `json:"working"`
	CurrentTurnStarted *time.Time `json:"currentTurnStarted,omitempty"`
	LastActivityAt     time.Time  `json:"lastActivityAt"`
	AsyncWork          int        `json:"asyncWork"`
	LedgerCursor       int        `json:"ledgerCursor"`
}

// chat writes c's history: its creation at the root, its placement, then a
// turn, so the log holds real /parentId, /order and turn-state patches.
func (l *legacy) chat(t *testing.T, id, workspaceID, parentID string, order int) {
	t.Helper()
	born := schema1Chat{legacyChat: legacyChat{Chat: domain.Chat{ID: id, WorkspaceID: workspaceID, Type: domain.ChatTypeChat}}}
	born.LastActivityAt = time.Unix(100, 0).UTC()
	placed := born
	placed.ParentID, placed.Order = parentID, order
	started := placed
	at := time.Unix(200, 0).UTC()
	started.Working, started.CurrentTurnStarted, started.LastActivityAt, started.AsyncWork = true, &at, at, 1
	stopped := started
	stopped.Working, stopped.CurrentTurnStarted, stopped.AsyncWork, stopped.LedgerCursor = false, nil, 0, 3
	writeAll(t, l.chatLog, map[string][]schema1Chat{id: {born, placed, started, stopped}})
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
	assert.Empty(t, got.UnreadChats, "a turn's patches do not stop a history replaying")
	assert.Equal(t, []LegacyChat{{ID: "c-1", WorkspaceID: "ws-1", Placement: tree.Placement{ParentID: "f-1", Order: 3}}},
		got.Chats, "a schema-1 history still carries the placement it patched")
	assert.ElementsMatch(t, []tree.Node{
		{ID: "f-1", Kind: tree.KindFolder, Order: 2},
		{ID: "c-2", Kind: tree.KindChat, ParentID: "f-1"},
	}, got.Nodes)
	assert.Equal(t, []tree.Folder{{ID: "f-1", Name: "Ideas", HomeID: "h-1"}}, got.Folders)
}

// A build before home folders kept only repo folders, in a table with no home_id.
func TestReadLegacy_ReadsTheFoldersTableABuildBeforeHomeFoldersWrote(t *testing.T) {
	l := newLegacy(t)
	require.NoError(t, l.view.Exec("CREATE TABLE folders (id text PRIMARY KEY, repo_id text, project_id text, parent_id text, name text, `order` integer)").Error)
	require.NoError(t, l.view.Exec("INSERT INTO folders VALUES ('f-1', 'r-1', 'p-1', '', 'Ideas', 0)").Error)

	got, err := ReadLegacy(context.Background(), l.sources())

	require.NoError(t, err)
	assert.Equal(t, []tree.Folder{{ID: "f-1", Name: "Ideas", RepoID: "r-1"}}, got.Folders)
}

// The retire-legacy step deletes the node log only while it still matches the
// backup, so reading it must not add the tables the live opener creates.
func TestReadLegacy_LeavesTheNodeLogAsTheRetiredBuildWroteIt(t *testing.T) {
	l := newLegacy(t)
	l.nodes(t, legacyNode{ID: "f-1", Kind: "folder"})
	db, err := storesqlite.OpenDB(l.nodeLog())
	require.NoError(t, err)
	require.NoError(t, db.Exec("DROP TABLE receipts").Error)
	require.NoError(t, db.Exec("DROP TABLE deliveries").Error)
	require.NoError(t, db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	before, err := os.ReadFile(l.nodeLog())
	require.NoError(t, err)

	got, err := ReadLegacy(context.Background(), l.sources())

	require.NoError(t, err)
	assert.Equal(t, []tree.Node{{ID: "f-1", Kind: tree.KindFolder}}, got.Nodes)
	after, err := os.ReadFile(l.nodeLog())
	require.NoError(t, err)
	assert.Equal(t, before, after, "reading the retired node log leaves it byte for byte as it was")
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
