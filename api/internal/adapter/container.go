package adapter

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
	"github.com/char2cs/crowbar/api/internal/core/paths"
)

// per-entity DB file names (siblings inside a storages/ directory).
const (
	eventStreamDBName = "event_stream.db"
	viewDBName        = "view.db"
)

// per-type DB file names under state/events and state/store. One event log +
// one read-model DB per aggregate type, routed by shard hash across many ids.
const (
	workspaceDBName    = "workspace.db"
	reviewThreadDBName = "review_thread.db"
)

// Container is the persistence layer.
//
// The quiver-faithful per-type plane opens ONE event log + ONE read-model DB
// per aggregate type at boot: state/events/<type>.db (append-only truth) and
// state/store/<type>.db (durable read-model projection, opened as a read pool).
// One asynx instance per type routes many aggregate ids by shard hash.
//
// The per-entity workspace plane (per-workspace event_stream.db + view.db under
// <home>/projects/<P>/<R>/workspaces/<W>/storages, LRU-cached) is retained ONLY
// until its callers are converted in Task 7; it is deleted then. Chat keeps its
// global event store under <home>/state until Task 13. Projects, repositories,
// terminal profiles, and settings live in the global view.db under <home>/state.
type Container struct {
	crowbarHome string

	chatES         asynxModels.Store
	reviewThreadES asynxModels.Store

	// Per-type handles (quiver-faithful). The workspace ones are exposed under
	// temporary names (WorkspaceEventStore/WorkspaceStoreDB) because Go has no
	// method overloading and the 3-arg per-entity WorkspaceES/WorkspaceView still
	// live; Task 7 deletes the per-entity pair and promotes these to WorkspaceES()
	// / WorkspaceView().
	workspaceEventStore asynxModels.Store
	workspaceStoreDB    *gormdb.DB
	reviewThreadView    *gormdb.DB

	workspaceES   *Registry[asynxModels.Store]
	workspaceView *Registry[*gormdb.DB]

	globalView *gormdb.DB

	lock *instanceLock

	releaseMu sync.Mutex
	closed    bool

	globalClosers []io.Closer
}

// ErrClosed is returned by the per-entity accessors once Close has run, so a
// detached good-path-async goroutine cannot lazily re-open (and re-create the
// storages dir of) an entity DB after shutdown has begun.
var ErrClosed = errors.New("adapter: container closed")

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
// view DB, and the lazy per-entity workspace registries. It acquires the
// single-instance lock over the crowbar tree.
func New(
	opts ...Option,
) (*Container, error) {
	cfg := adapterOpts{}
	for _, o := range opts {
		o(&cfg)
	}

	home, stateDir := resolveHome(cfg.homeDir)
	if mkErr := os.MkdirAll(stateDir, 0o750); mkErr != nil {
		return nil, fmt.Errorf("adapter: state dir: %w", mkErr)
	}

	lock, err := acquireStateLock(stateDir)
	if err != nil {
		return nil, err
	}

	container, err := newLocked(home, stateDir, lock)
	if err != nil {
		_ = lock.Close()
		return nil, err
	}
	return container, nil
}

// resolveHome returns the crowbar home root and the global state directory. When
// homeDir is empty the production location is used; the home root is derived as
// the parent of the resolved state directory.
func resolveHome(
	homeDir string,
) (string, string) {
	if homeDir != "" {
		return homeDir, metadata.GetStateDirPathAt(homeDir)
	}
	stateDir := metadata.GetStateDirPath()
	return filepath.Dir(stateDir), stateDir
}

func newLocked(
	home string,
	stateDir string,
	lock *instanceLock,
) (c *Container, err error) {
	// Roll back every DB opened so far if any later open fails; on success err is
	// nil and the container owns them (closed in Close).
	var rollback []func() error
	defer func() {
		if err != nil {
			for i := len(rollback) - 1; i >= 0; i-- {
				_ = rollback[i]()
			}
		}
	}()

	chatES, err := eventsqlite.NewEventStore(filepath.Join(stateDir, "chat_"+eventStreamDBName))
	if err != nil {
		return nil, fmt.Errorf("adapter: chat event store: %w", err)
	}
	rollback = append(rollback, func() error { return closeEventStore(chatES) })

	// Per-type planes derive from the resolved home via the home-parameterized
	// path accessors (never paths.Events()/Store(), which are blind to cfg.homeDir
	// and would leak state into the prod ~/.crowbar — decision 14).
	eventsDir, err := paths.EventsAt(home)
	if err != nil {
		return nil, fmt.Errorf("adapter: events dir: %w", err)
	}
	storeDir, err := paths.StoreAt(home)
	if err != nil {
		return nil, fmt.Errorf("adapter: store dir: %w", err)
	}

	reviewThreadES, err := eventsqlite.NewEventStore(filepath.Join(eventsDir, reviewThreadDBName))
	if err != nil {
		return nil, fmt.Errorf("adapter: review thread event store: %w", err)
	}
	rollback = append(rollback, func() error { return closeEventStore(reviewThreadES) })

	workspaceEventStore, err := eventsqlite.NewEventStore(filepath.Join(eventsDir, workspaceDBName))
	if err != nil {
		return nil, fmt.Errorf("adapter: workspace event store: %w", err)
	}
	rollback = append(rollback, func() error { return closeEventStore(workspaceEventStore) })

	workspaceStoreDB, err := storesqlite.OpenReadPoolDB(filepath.Join(storeDir, workspaceDBName))
	if err != nil {
		return nil, fmt.Errorf("adapter: workspace store db: %w", err)
	}
	rollback = append(rollback, func() error { return closeViewDB(workspaceStoreDB) })

	reviewThreadView, err := storesqlite.OpenReadPoolDB(filepath.Join(storeDir, reviewThreadDBName))
	if err != nil {
		return nil, fmt.Errorf("adapter: review thread view: %w", err)
	}
	rollback = append(rollback, func() error { return closeViewDB(reviewThreadView) })

	globalView, err := storesqlite.OpenReadPoolDB(filepath.Join(stateDir, viewDBName))
	if err != nil {
		return nil, fmt.Errorf("adapter: global view: %w", err)
	}
	rollback = append(rollback, func() error { return closeViewDB(globalView) })

	c = &Container{
		crowbarHome:         home,
		chatES:              chatES,
		reviewThreadES:      reviewThreadES,
		workspaceEventStore: workspaceEventStore,
		workspaceStoreDB:    workspaceStoreDB,
		reviewThreadView:    reviewThreadView,
		globalView:          globalView,
		lock:                lock,
	}
	c.globalClosers = collectClosers(chatES, reviewThreadES)
	c.workspaceES = NewRegistry[asynxModels.Store](maxOpenEntityDBs, closeEventStore)
	c.workspaceView = NewRegistry[*gormdb.DB](maxOpenEntityDBs, closeViewDB)
	return c, nil
}

// ChatES returns the global chat event store.
func (c *Container) ChatES() asynxModels.Store {
	return c.chatES
}

// ReviewThreadES returns the reviewthread event log at state/events/review_thread.db.
func (c *Container) ReviewThreadES() asynxModels.Store {
	return c.reviewThreadES
}

// ReviewThreadView returns the reviewthread read-model DB at
// state/store/review_thread.db, opened as a read pool. Task 12 wires the
// reviewthread store/hub projections onto it (its read model moves out of the
// shared view.db).
func (c *Container) ReviewThreadView() *gormdb.DB {
	return c.reviewThreadView
}

// WorkspaceEventStore returns the workspace event log at
// state/events/workspace.db. This is the singleton per-type handle; Task 7
// promotes it to WorkspaceES() once the 3-arg per-entity WorkspaceES is deleted.
func (c *Container) WorkspaceEventStore() asynxModels.Store {
	return c.workspaceEventStore
}

// WorkspaceStoreDB returns the workspace read-model DB at
// state/store/workspace.db, opened as a read pool. Task 7 promotes it to
// WorkspaceView() once the 3-arg per-entity WorkspaceView is deleted.
func (c *Container) WorkspaceStoreDB() *gormdb.DB {
	return c.workspaceStoreDB
}

// GlobalView returns the global view DB at state/view.db holding projects,
// repositories, terminal profiles, and settings read models. It is a view DB, so
// it is opened as a read pool (decision 12) — never the single-conn OpenDB that
// would head-of-line-block reads.
func (c *Container) GlobalView() *gormdb.DB {
	return c.globalView
}

// CrowbarHome returns the resolved crowbar home root this container was opened
// against. Path-deriving usecases (worktree add, project import/delete) MUST use
// this same home so the git worktrees and the per-entity storages land under one
// root — otherwise an overridden home (e.g. a test temp dir) splits them.
func (c *Container) CrowbarHome() string {
	return c.crowbarHome
}

// isClosed reports whether Close has run. The per-entity accessors check it so a
// detached good-path-async goroutine cannot re-open an entity DB after shutdown.
func (c *Container) isClosed() bool {
	c.releaseMu.Lock()
	defer c.releaseMu.Unlock()
	return c.closed
}

// WorkspaceES returns the per-entity workspace event store, lazily creating the
// storages directory and opening event_stream.db on first access. The handle is
// pinned in the registry until the returned release func is called; the caller
// (the owning workspace entity) must call it when that entity is evicted, so the
// LRU can reclaim the handle. release is nil on the error paths.
func (c *Container) WorkspaceES(
	projectID string,
	repoID string,
	wsID string,
) (asynxModels.Store, func(), error) {
	if c.isClosed() {
		return nil, nil, ErrClosed
	}
	dir := workspaceStorageDir(c.crowbarHome, projectID, repoID, wsID)
	key := entityKey(projectID, repoID, wsID)
	es, release, err := c.workspaceES.Acquire(key, func() (asynxModels.Store, error) {
		if mkErr := os.MkdirAll(dir, 0o750); mkErr != nil {
			return nil, fmt.Errorf("adapter: workspace storages dir: %w", mkErr)
		}
		store, openErr := eventsqlite.NewEventStore(filepath.Join(dir, eventStreamDBName))
		if openErr != nil {
			return nil, fmt.Errorf("adapter: workspace event store: %w", openErr)
		}
		return store, nil
	})
	if err != nil {
		return nil, nil, err
	}
	return es, release, nil
}

// WorkspaceView returns the per-entity workspace view DB, lazily creating the
// storages directory and opening view.db on first access. The handle is pinned
// in the registry until the returned release func is called; the caller (the
// owning workspace entity) must call it when that entity is evicted, so the LRU
// can reclaim the handle. release is nil on the error paths.
func (c *Container) WorkspaceView(
	projectID string,
	repoID string,
	wsID string,
) (*gormdb.DB, func(), error) {
	if c.isClosed() {
		return nil, nil, ErrClosed
	}
	dir := workspaceStorageDir(c.crowbarHome, projectID, repoID, wsID)
	key := entityKey(projectID, repoID, wsID)
	view, release, err := c.workspaceView.Acquire(key, func() (*gormdb.DB, error) {
		if mkErr := os.MkdirAll(dir, 0o750); mkErr != nil {
			return nil, fmt.Errorf("adapter: workspace storages dir: %w", mkErr)
		}
		db, openErr := storesqlite.OpenDB(filepath.Join(dir, viewDBName))
		if openErr != nil {
			return nil, fmt.Errorf("adapter: workspace view: %w", openErr)
		}
		return db, nil
	})
	if err != nil {
		return nil, nil, err
	}
	return view, release, nil
}

// Close drains and closes every persistence plane — the per-type event/read
// handles, the cached per-entity handles, the global view DB, and the global
// event stores — then releases the single-instance lock. Closing a WAL DB's last
// connection checkpoints its WAL, so errors.Join spans all planes with the WAL
// checkpointed.
func (c *Container) Close() error {
	var errs []error

	c.releaseMu.Lock()
	c.closed = true
	c.releaseMu.Unlock()

	if c.workspaceES != nil {
		if err := c.workspaceES.CloseAll(); err != nil {
			errs = append(errs, fmt.Errorf("adapter: close workspace event stores: %w", err))
		}
	}
	if c.workspaceView != nil {
		if err := c.workspaceView.CloseAll(); err != nil {
			errs = append(errs, fmt.Errorf("adapter: close workspace views: %w", err))
		}
	}

	if c.workspaceStoreDB != nil {
		if err := closeViewDB(c.workspaceStoreDB); err != nil {
			errs = append(errs, fmt.Errorf("adapter: close workspace store db: %w", err))
		}
		c.workspaceStoreDB = nil
	}
	if c.reviewThreadView != nil {
		if err := closeViewDB(c.reviewThreadView); err != nil {
			errs = append(errs, fmt.Errorf("adapter: close review thread view: %w", err))
		}
		c.reviewThreadView = nil
	}

	if c.globalView != nil {
		if err := closeViewDB(c.globalView); err != nil {
			errs = append(errs, fmt.Errorf("adapter: close global view: %w", err))
		}
		c.globalView = nil
	}

	if c.workspaceEventStore != nil {
		if err := closeEventStore(c.workspaceEventStore); err != nil {
			errs = append(errs, fmt.Errorf("adapter: close workspace event store: %w", err))
		}
		c.workspaceEventStore = nil
	}

	for _, cl := range c.globalClosers {
		if err := cl.Close(); err != nil {
			errs = append(errs, fmt.Errorf("adapter: close global event store: %w", err))
		}
	}
	c.globalClosers = nil

	if c.lock != nil {
		if err := c.lock.Close(); err != nil {
			errs = append(errs, fmt.Errorf("adapter: release state lock: %w", err))
		}
		c.lock = nil
	}
	return errors.Join(errs...)
}

func entityKey(
	parts ...string,
) string {
	return filepath.ToSlash(filepath.Join(parts...))
}

func workspaceStorageDir(
	home string,
	projectID string,
	repoID string,
	wsID string,
) string {
	return filepath.Join(
		home,
		"projects",
		projectID,
		repoID,
		"workspaces",
		wsID,
		"storages",
	)
}

func closeEventStore(
	es asynxModels.Store,
) error {
	if cl, ok := es.(io.Closer); ok {
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

func closeIfCloser(
	v any,
) {
	if cl, ok := v.(io.Closer); ok {
		_ = cl.Close()
	}
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
