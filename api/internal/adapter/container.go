package adapter

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/core/home"
)

// Global-plane DB file name under <home>/state: the non-aggregate view.db.
const (
	viewDBName = "view.db"
)

// per-type DB file names under state/events and state/store. One event log +
// one read-model DB per aggregate type, routed by shard hash across many ids.
const (
	workspaceDBName    = "workspace.db"
	reviewThreadDBName = "review_thread.db"
	agentChatDBName    = "agent_chat.db"
	treeDBName         = "tree.db"
)

// Per-type snapshot DB file names under state/events. Each is its own file and
// connection, never a table in the event log's single-writer db; see
// eventsqlite.NewSnapshotStore for the locking rationale.
const (
	workspaceSnapshotDBName    = "workspace_snapshots.db"
	reviewThreadSnapshotDBName = "review_thread_snapshots.db"
	agentChatSnapshotDBName    = "agent_chat_snapshots.db"
	treeSnapshotDBName         = "tree_snapshots.db"
)

// Container is the persistence layer. Per aggregate type it opens one append-only
// event log (state/events/<type>.db) and one read-model DB (state/store/<type>.db,
// a read pool); projects, repositories, terminal profiles and settings share view.db.
type Container struct {
	home home.Home

	reviewThreadES asynxModels.Store
	reviewThreadSS asynxModels.SnapshotStore

	// Per-type handles: one event log, one snapshot store (one upserted snapshot per
	// aggregate, keyed by aggregateID) and one read-model DB per aggregate type.
	workspaceEventStore    asynxModels.Store
	workspaceSnapshotStore asynxModels.SnapshotStore
	workspaceStoreDB       *gormdb.DB
	reviewThreadView       *gormdb.DB
	agentChatEventStore    asynxModels.Store
	agentChatSnapshotStore asynxModels.SnapshotStore
	agentChatStoreDB       *gormdb.DB
	// The Tree has no read model: Get is its read.
	treeEventStore    asynxModels.Store
	treeSnapshotStore asynxModels.SnapshotStore
	chatLedgerDB      *gormdb.DB

	globalView *gormdb.DB

	globalClosers []io.Closer
}

type adapterOpts struct {
	homeDir string
}

// Option configures adapter.New.
type Option func(*adapterOpts)

// WithHomeDir overrides the home directory used for path resolution.
func WithHomeDir(
	dir string,
) Option {
	return func(o *adapterOpts) {
		o.homeDir = dir
	}
}

// New constructs the persistence layer: the global event stores, the global
// view DB, and the lazy per-entity workspace registries. It takes no lock: the
// caller holds the home through its migration session (migrate.Begin).
func New(
	opts ...Option,
) (*Container, error) {
	cfg := adapterOpts{}
	for _, o := range opts {
		o(&cfg)
	}

	h, err := home.ResolveDir(cfg.homeDir)
	if err != nil {
		return nil, fmt.Errorf("adapter: home: %w", err)
	}
	// Root first, owner-only; events and store sit under state, so this creates all three.
	for _, d := range []struct {
		dir  string
		perm os.FileMode
	}{{h.Root(), 0o700}, {h.Events(), 0o750}, {h.Store(), 0o750}} {
		if mkErr := os.MkdirAll(d.dir, d.perm); mkErr != nil {
			return nil, fmt.Errorf("adapter: create %s: %w", d.dir, mkErr)
		}
	}

	return openContainer(h)
}

func openContainer(
	h home.Home,
) (c *Container, err error) {
	eventsDir, storeDir := h.Events(), h.Store()

	// o keeps the first error and the rollback for everything opened before it; later
	// opens are no-ops after a failure, so this reads as a list, not error branches.
	var o opens
	defer func() {
		if err != nil {
			err = errors.Join(err, o.unwind())
		}
	}()

	reviewThreadES := o.eventStore(eventsDir, reviewThreadDBName, "review thread event store")
	reviewThreadSS := o.snapshotStore(eventsDir, reviewThreadSnapshotDBName, "review thread snapshot store")
	workspaceEventStore := o.eventStore(eventsDir, workspaceDBName, "workspace event store")
	workspaceSnapshotStore := o.snapshotStore(eventsDir, workspaceSnapshotDBName, "workspace snapshot store")
	workspaceStoreDB := o.viewDB(storeDir, workspaceDBName, "workspace store db")
	reviewThreadView := o.viewDB(storeDir, reviewThreadDBName, "review thread view")
	agentChatEventStore := o.eventStore(eventsDir, agentChatDBName, "agent chat event store")
	agentChatSnapshotStore := o.snapshotStore(eventsDir, agentChatSnapshotDBName, "agent chat snapshot store")
	agentChatStoreDB := o.viewDB(storeDir, agentChatDBName, "agent chat store db")
	treeEventStore := o.eventStore(eventsDir, treeDBName, "tree event store")
	treeSnapshotStore := o.snapshotStore(eventsDir, treeSnapshotDBName, "tree snapshot store")
	chatLedgerDB := o.chatLedgerDB(h)
	globalView := o.viewDB(h.State(), viewDBName, "global view")

	if o.err != nil {
		err = o.err
		return nil, err
	}

	c = &Container{
		home:                   h,
		reviewThreadES:         reviewThreadES,
		reviewThreadSS:         reviewThreadSS,
		workspaceEventStore:    workspaceEventStore,
		workspaceSnapshotStore: workspaceSnapshotStore,
		workspaceStoreDB:       workspaceStoreDB,
		reviewThreadView:       reviewThreadView,
		agentChatEventStore:    agentChatEventStore,
		agentChatSnapshotStore: agentChatSnapshotStore,
		agentChatStoreDB:       agentChatStoreDB,
		treeEventStore:         treeEventStore,
		treeSnapshotStore:      treeSnapshotStore,
		chatLedgerDB:           chatLedgerDB,
		globalView:             globalView,
	}
	c.globalClosers = collectClosers(reviewThreadES, reviewThreadSS)
	return c, nil
}

// opens is a fail-fast opener for openContainer: it keeps the first error and the
// rollback for everything opened before it. After a failure later opens are
// no-ops, so the container is never built from a half-open set.
type opens struct {
	err      error
	rollback []func() error
}

// unwind closes what was opened, newest first. Only called when openContainer is
// returning an error — on success the Container owns these and Close has them.
func (o *opens) unwind() error {
	var errs []error
	for i := len(o.rollback) - 1; i >= 0; i-- {
		errs = append(errs, o.rollback[i]())
	}
	return errors.Join(errs...)
}

func (o *opens) eventStore(dir, name, what string) asynxModels.Store {
	return openOne(o, what, func() (asynxModels.Store, error) {
		return eventsqlite.NewEventStore(filepath.Join(dir, name))
	}, closeEventStore)
}

func (o *opens) snapshotStore(dir, name, what string) asynxModels.SnapshotStore {
	return openOne(o, what, func() (asynxModels.SnapshotStore, error) {
		return eventsqlite.NewSnapshotStore(filepath.Join(dir, name))
	}, closeSnapshotStore)
}

func (o *opens) viewDB(dir, name, what string) *gormdb.DB {
	return openOne(o, what, func() (*gormdb.DB, error) {
		return storesqlite.OpenReadPoolDB(filepath.Join(dir, name))
	}, closeViewDB)
}

func openOne[T any](o *opens, what string, open func() (T, error), closeFn func(T) error) T {
	var zero T
	if o.err != nil {
		return zero
	}
	v, err := open()
	if err != nil {
		o.err = fmt.Errorf("adapter: %s: %w", what, err)
		return zero
	}
	o.rollback = append(o.rollback, func() error { return closeFn(v) })
	return v
}

// ReviewThreadES returns the reviewthread event log at state/events/review_thread.db.
func (c *Container) ReviewThreadES() asynxModels.Store {
	return c.reviewThreadES
}

// ReviewThreadSS returns the reviewthread snapshot store at
// state/events/review_thread_snapshots.db — the asynx v0.8 O(1) warm-read cache
// paired with ReviewThreadES.
func (c *Container) ReviewThreadSS() asynxModels.SnapshotStore {
	return c.reviewThreadSS
}

// ReviewThreadView returns the reviewthread read-model DB at
// state/store/review_thread.db, opened as a read pool.
func (c *Container) ReviewThreadView() *gormdb.DB {
	return c.reviewThreadView
}

// WorkspaceES returns the workspace event log at state/events/workspace.db. This
// is the singleton per-type handle: the app layer builds ONE axWorkspace over it
// and routes every workspace id to a shard by hash.
func (c *Container) WorkspaceES() asynxModels.Store {
	return c.workspaceEventStore
}

// WorkspaceSS returns the workspace snapshot store at state/events/workspace_snapshots.db,
// the O(1) warm-read cache paired with WorkspaceES on the workspace-scoped read path.
func (c *Container) WorkspaceSS() asynxModels.SnapshotStore {
	return c.workspaceSnapshotStore
}

// WorkspaceView returns the workspace read-model DB at state/store/workspace.db,
// opened as a read pool. The workspace store projection folds
// evt.Aggregate into it, and it doubles as the location index.
func (c *Container) WorkspaceView() *gormdb.DB {
	return c.workspaceStoreDB
}

// AgentChatES returns the agent-chat event log at state/events/agent_chat.db. The app
// layer builds one axAgentChat over it and routes every chat id to a shard by hash.
func (c *Container) AgentChatES() asynxModels.Store {
	return c.agentChatEventStore
}

// AgentChatSS returns the agent-chat snapshot store at
// state/events/agent_chat_snapshots.db — the asynx v0.8 O(1) warm-read cache
// paired with AgentChatES.
func (c *Container) AgentChatSS() asynxModels.SnapshotStore {
	return c.agentChatSnapshotStore
}

// AgentChatReadDB returns the agent-chat read-model DB at state/store/agent_chat.db,
// opened as a read pool; the agentchat store projection folds into it.
func (c *Container) AgentChatReadDB() *gormdb.DB {
	return c.agentChatStoreDB
}

// TreeES returns the Tree event log at state/events/tree.db.
func (c *Container) TreeES() asynxModels.Store {
	return c.treeEventStore
}

// TreeSS returns the Tree snapshot store at state/events/tree_snapshots.db.
func (c *Container) TreeSS() asynxModels.SnapshotStore {
	return c.treeSnapshotStore
}

// GlobalView returns the global view DB at state/view.db (projects, repositories,
// terminal profiles, settings), opened as a read pool so reads never queue behind
// a single connection.
func (c *Container) GlobalView() *gormdb.DB {
	return c.globalView
}

// Home is the Crowbar home this container was opened against. Path-deriving
// usecases (worktree add, project import/delete) MUST use this same home so the
// git worktrees and the per-entity storages land under one root.
func (c *Container) Home() home.Home {
	return c.home
}

// Close drains and closes every persistence plane — the per-type event/read
// handles, the global view DB, and the global event stores. Closing a WAL DB's
// last connection checkpoints its WAL, so errors.Join spans all planes.
func (c *Container) Close() error {
	var errs []error

	// A list, not a flow: each entry skips a handle never built, closes it and
	// forgets it, so adding a store is one line that cannot forget to nil it.
	closeEach(&errs, "workspace store db", &c.workspaceStoreDB, closeViewDB)
	closeEach(&errs, "review thread view", &c.reviewThreadView, closeViewDB)
	closeEach(&errs, "agent chat store db", &c.agentChatStoreDB, closeViewDB)
	closeEach(&errs, "chat ledger db", &c.chatLedgerDB, closeViewDB)
	closeEach(&errs, "global view", &c.globalView, closeViewDB)

	closeEach(&errs, "workspace event store", &c.workspaceEventStore, closeEventStore)
	closeEach(&errs, "workspace snapshot store", &c.workspaceSnapshotStore, closeSnapshotStore)
	closeEach(&errs, "agent chat event store", &c.agentChatEventStore, closeEventStore)
	closeEach(&errs, "agent chat snapshot store", &c.agentChatSnapshotStore, closeSnapshotStore)
	closeEach(&errs, "tree event store", &c.treeEventStore, closeEventStore)
	closeEach(&errs, "tree snapshot store", &c.treeSnapshotStore, closeSnapshotStore)

	for _, cl := range c.globalClosers {
		if err := cl.Close(); err != nil {
			errs = append(errs, fmt.Errorf("adapter: close global event store: %w", err))
		}
	}
	c.globalClosers = nil
	return errors.Join(errs...)
}

// closeEach closes one handle and clears it, collecting a named error. A zero
// handle is skipped, so Close works on a partially built container after a failed
// boot; clearing the field makes Close idempotent.
func closeEach[T comparable](errs *[]error, what string, field *T, close func(T) error) {
	var zero T
	if *field == zero {
		return
	}
	if err := close(*field); err != nil {
		*errs = append(*errs, fmt.Errorf("adapter: close %s: %w", what, err))
	}
	*field = zero
}

func closeEventStore(
	es asynxModels.Store,
) error {
	if cl, ok := es.(io.Closer); ok {
		return cl.Close()
	}
	return nil
}

func closeSnapshotStore(
	ss asynxModels.SnapshotStore,
) error {
	if cl, ok := ss.(io.Closer); ok {
		return cl.Close()
	}
	return nil
}

func closeViewDB(
	db *gormdb.DB,
) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("adapter: db handle: %w", err)
	}
	return sqlDB.Close()
}

func collectClosers(
	values ...any,
) []io.Closer {
	closers := make([]io.Closer, 0, len(values))
	for _, v := range values {
		if cl, ok := v.(io.Closer); ok {
			closers = append(closers, cl)
		}
	}
	return closers
}
