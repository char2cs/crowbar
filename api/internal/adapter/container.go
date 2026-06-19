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
)

// per-entity DB file names (siblings inside a storages/ directory).
const (
	eventStreamDBName = "event_stream.db"
	viewDBName        = "view.db"
)

// Container is the persistence layer. The workspace aggregate is event-sourced
// per entity: each workspace owns its own event_stream.db + view.db under
// <home>/projects/<P>/<R>/workspaces/<W>/storages, opened lazily and cached in a
// ref-counted LRU registry. The chat and reviewthread aggregates keep their
// global event stores under <home>/state. Projects, repositories, terminal
// profiles, and settings live in the global view.db (also under <home>/state).
type Container struct {
	crowbarHome string

	chatES         asynxModels.Store
	reviewThreadES asynxModels.Store

	workspaceES   *Registry[asynxModels.Store]
	workspaceView *Registry[*gormdb.DB]

	globalView *gormdb.DB

	lock *instanceLock

	releaseMu sync.Mutex
	releases  []func()

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
) (*Container, error) {
	chatES, err := eventsqlite.NewEventStore(filepath.Join(stateDir, "chat_"+eventStreamDBName))
	if err != nil {
		return nil, fmt.Errorf("adapter: chat event store: %w", err)
	}
	reviewThreadES, err := eventsqlite.NewEventStore(filepath.Join(stateDir, "review_thread_"+eventStreamDBName))
	if err != nil {
		closeIfCloser(chatES)
		return nil, fmt.Errorf("adapter: review thread event store: %w", err)
	}

	globalView, err := storesqlite.OpenDB(filepath.Join(stateDir, viewDBName))
	if err != nil {
		closeIfCloser(chatES)
		closeIfCloser(reviewThreadES)
		return nil, fmt.Errorf("adapter: global view: %w", err)
	}

	c := &Container{
		crowbarHome:    home,
		chatES:         chatES,
		reviewThreadES: reviewThreadES,
		globalView:     globalView,
		lock:           lock,
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

// ReviewThreadES returns the global reviewthread event store.
func (c *Container) ReviewThreadES() asynxModels.Store {
	return c.reviewThreadES
}

// GlobalView returns the global view DB holding projects, repositories,
// terminal profiles, and settings read models.
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

// WorkspaceES returns the per-entity workspace event store, lazily creating the
// storages directory and opening event_stream.db on first access. The handle is
// pinned for the process lifetime and released on Close.
func (c *Container) WorkspaceES(
	projectID string,
	repoID string,
	wsID string,
) (asynxModels.Store, error) {
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
		return nil, err
	}
	c.pin(release)
	return es, nil
}

// WorkspaceView returns the per-entity workspace view DB, lazily creating the
// storages directory and opening view.db on first access. The handle is pinned
// for the process lifetime and released on Close.
func (c *Container) WorkspaceView(
	projectID string,
	repoID string,
	wsID string,
) (*gormdb.DB, error) {
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
		return nil, err
	}
	c.pin(release)
	return view, nil
}

// Close drains and closes every cached per-entity handle, the global view DB,
// the global event stores, and finally releases the single-instance lock.
func (c *Container) Close() error {
	var errs []error

	c.releaseMu.Lock()
	releases := c.releases
	c.releases = nil
	c.releaseMu.Unlock()
	for _, release := range releases {
		release()
	}

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

	if c.globalView != nil {
		if err := closeViewDB(c.globalView); err != nil {
			errs = append(errs, fmt.Errorf("adapter: close global view: %w", err))
		}
		c.globalView = nil
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

func (c *Container) pin(
	release func(),
) {
	c.releaseMu.Lock()
	c.releases = append(c.releases, release)
	c.releaseMu.Unlock()
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
