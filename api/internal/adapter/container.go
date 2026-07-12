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
	"github.com/char2cs/crowbar/api/internal/core/metadata"
	"github.com/char2cs/crowbar/api/internal/core/paths"
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
	agentRunnerDBName  = "agent_runner.db"
)

// Container is the persistence layer.
//
// The quiver-faithful per-type plane opens ONE event log + ONE read-model DB
// per aggregate type at boot: state/events/<type>.db (append-only truth) and
// state/store/<type>.db (durable read-model projection, opened as a read pool).
// One asynx instance per type routes many aggregate ids by shard hash.
//
// Projects, repositories, terminal profiles, and settings live in the global
// view.db under <home>/state.
type Container struct {
	crowbarHome string

	reviewThreadES asynxModels.Store

	// Per-type handles (quiver-faithful): one event log + one read-model DB per
	// aggregate type, routed to many aggregate ids by shard hash.
	workspaceEventStore   asynxModels.Store
	workspaceStoreDB      *gormdb.DB
	reviewThreadView      *gormdb.DB
	agentChatEventStore   asynxModels.Store
	agentChatStoreDB      *gormdb.DB
	agentRunnerEventStore asynxModels.Store
	agentRunnerStoreDB    *gormdb.DB

	globalView *gormdb.DB

	lock *instanceLock

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

	agentChatEventStore, err := eventsqlite.NewEventStore(filepath.Join(eventsDir, agentChatDBName))
	if err != nil {
		return nil, fmt.Errorf("adapter: agent chat event store: %w", err)
	}
	rollback = append(rollback, func() error { return closeEventStore(agentChatEventStore) })

	agentChatStoreDB, err := storesqlite.OpenReadPoolDB(filepath.Join(storeDir, agentChatDBName))
	if err != nil {
		return nil, fmt.Errorf("adapter: agent chat store db: %w", err)
	}
	rollback = append(rollback, func() error { return closeViewDB(agentChatStoreDB) })

	agentRunnerEventStore, err := eventsqlite.NewEventStore(filepath.Join(eventsDir, agentRunnerDBName))
	if err != nil {
		return nil, fmt.Errorf("adapter: agent runner event store: %w", err)
	}
	rollback = append(rollback, func() error { return closeEventStore(agentRunnerEventStore) })

	agentRunnerStoreDB, err := storesqlite.OpenReadPoolDB(filepath.Join(storeDir, agentRunnerDBName))
	if err != nil {
		return nil, fmt.Errorf("adapter: agent runner store db: %w", err)
	}
	rollback = append(rollback, func() error { return closeViewDB(agentRunnerStoreDB) })

	globalView, err := storesqlite.OpenReadPoolDB(filepath.Join(stateDir, viewDBName))
	if err != nil {
		return nil, fmt.Errorf("adapter: global view: %w", err)
	}
	rollback = append(rollback, func() error { return closeViewDB(globalView) })

	c = &Container{
		crowbarHome:           home,
		reviewThreadES:        reviewThreadES,
		workspaceEventStore:   workspaceEventStore,
		workspaceStoreDB:      workspaceStoreDB,
		reviewThreadView:      reviewThreadView,
		agentChatEventStore:   agentChatEventStore,
		agentChatStoreDB:      agentChatStoreDB,
		agentRunnerEventStore: agentRunnerEventStore,
		agentRunnerStoreDB:    agentRunnerStoreDB,
		globalView:            globalView,
		lock:                  lock,
	}
	c.globalClosers = collectClosers(reviewThreadES)
	return c, nil
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

// WorkspaceES returns the workspace event log at state/events/workspace.db. This
// is the singleton per-type handle: the app layer builds ONE axWorkspace over it
// and routes every workspace id to a shard by hash (spec §3.3/§3.4).
func (c *Container) WorkspaceES() asynxModels.Store {
	return c.workspaceEventStore
}

// WorkspaceView returns the workspace read-model DB at state/store/workspace.db,
// opened as a read pool (decision 12). The workspace store projection folds
// evt.Aggregate into it, and it doubles as the location index (spec §3.7).
func (c *Container) WorkspaceView() *gormdb.DB {
	return c.workspaceStoreDB
}

// AgentChatES returns the agent-chat event log at state/events/agent_chat.db.
// This is the singleton per-type handle: the app layer builds ONE axAgentChat
// over it and routes every chat id to a shard by hash, mirroring
// WorkspaceES/ReviewThreadES.
func (c *Container) AgentChatES() asynxModels.Store {
	return c.agentChatEventStore
}

// AgentChatReadDB returns the agent-chat read-model DB at
// state/store/agent_chat.db, opened as a read pool (decision 12). The
// agentchat store projection folds evt.Aggregate into it, mirroring
// WorkspaceView/ReviewThreadView.
func (c *Container) AgentChatReadDB() *gormdb.DB {
	return c.agentChatStoreDB
}

// AgentRunnerES returns the agent-runner event log at
// state/events/agent_runner.db. This is the singleton per-type handle: the app
// layer builds ONE axAgentRunner over it and routes every runner id to a shard by
// hash, mirroring WorkspaceES/ReviewThreadES/AgentChatES.
func (c *Container) AgentRunnerES() asynxModels.Store {
	return c.agentRunnerEventStore
}

// AgentRunnerReadDB returns the agent-runner read-model DB at
// state/store/agent_runner.db, opened as a read pool (decision 12). It holds BOTH
// runner projections: the live-runner model (rows exist exactly while their CLI
// runs — no status column, so nothing can go stale) and the append-only
// conversation history, mirroring AgentChatReadDB/WorkspaceView.
func (c *Container) AgentRunnerReadDB() *gormdb.DB {
	return c.agentRunnerStoreDB
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

// Close drains and closes every persistence plane — the per-type event/read
// handles, the global view DB, and the global event stores — then releases the
// single-instance lock. Closing a WAL DB's last connection checkpoints its WAL,
// so errors.Join spans all planes with the WAL checkpointed.
func (c *Container) Close() error {
	var errs []error

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
	if c.agentChatStoreDB != nil {
		if err := closeViewDB(c.agentChatStoreDB); err != nil {
			errs = append(errs, fmt.Errorf("adapter: close agent chat store db: %w", err))
		}
		c.agentChatStoreDB = nil
	}
	if c.agentRunnerStoreDB != nil {
		if err := closeViewDB(c.agentRunnerStoreDB); err != nil {
			errs = append(errs, fmt.Errorf("adapter: close agent runner store db: %w", err))
		}
		c.agentRunnerStoreDB = nil
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
	if c.agentChatEventStore != nil {
		if err := closeEventStore(c.agentChatEventStore); err != nil {
			errs = append(errs, fmt.Errorf("adapter: close agent chat event store: %w", err))
		}
		c.agentChatEventStore = nil
	}
	if c.agentRunnerEventStore != nil {
		if err := closeEventStore(c.agentRunnerEventStore); err != nil {
			errs = append(errs, fmt.Errorf("adapter: close agent runner event store: %w", err))
		}
		c.agentRunnerEventStore = nil
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
