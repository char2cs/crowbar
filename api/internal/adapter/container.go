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
	workspaceDBName     = "workspace.db"
	reviewThreadDBName  = "review_thread.db"
	agentChatDBName     = "agent_chat.db"
	agentRunnerDBName   = "agent_runner.db"
	agentActivityDBName = "agent_activity.db"
)

// per-type snapshot DB file names under state/events, one per aggregate type,
// each its OWN file+connection (never a table in the event log's single-writer
// db) — see eventsqlite.NewSnapshotStore for the locking rationale. asynx v0.8
// keeps exactly one upserted snapshot per aggregate here, read in O(1) on the
// workspace-scoped hot path.
const (
	workspaceSnapshotDBName     = "workspace_snapshots.db"
	reviewThreadSnapshotDBName  = "review_thread_snapshots.db"
	agentChatSnapshotDBName     = "agent_chat_snapshots.db"
	agentRunnerSnapshotDBName   = "agent_runner_snapshots.db"
	agentActivitySnapshotDBName = "agent_activity_snapshots.db"
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
	reviewThreadSS asynxModels.SnapshotStore

	// Per-type handles (quiver-faithful): one event log + one snapshot store +
	// one read-model DB per aggregate type, routed to many aggregate ids by
	// shard hash. The snapshot store is the asynx v0.8 O(1) warm-read cache
	// (one upserted snapshot per aggregate), keyed by aggregateID alone.
	workspaceEventStore    asynxModels.Store
	workspaceSnapshotStore asynxModels.SnapshotStore
	workspaceStoreDB       *gormdb.DB
	reviewThreadView       *gormdb.DB
	agentChatEventStore    asynxModels.Store
	agentChatSnapshotStore asynxModels.SnapshotStore
	agentChatStoreDB       *gormdb.DB
	// The activity plane is its OWN event log, snapshot store and read model.
	// Never a table inside the chat plane: chat events are a handful per chat
	// while activity events are hundreds per turn, and sharing one single-writer
	// event DB would put the sidebar's writes behind a tool-call storm.
	agentActivityEventStore    asynxModels.Store
	agentActivitySnapshotStore asynxModels.SnapshotStore
	agentActivityStoreDB       *gormdb.DB
	agentRunnerEventStore      asynxModels.Store
	agentRunnerSnapshotStore   asynxModels.SnapshotStore
	agentRunnerStoreDB         *gormdb.DB

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

	// Opening is a LIST, and o carries the two things every entry shares: the first
	// error, and the rollback for everything opened before it. After the first
	// failure every later open is a no-op, so the sequence reads as a declaration of
	// what this container is made of rather than as twenty error branches.
	var o opens
	defer func() {
		if err != nil {
			o.unwind()
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
	agentActivityEventStore := o.eventStore(eventsDir, agentActivityDBName, "agent activity event store")
	agentActivitySnapshotStore := o.snapshotStore(eventsDir, agentActivitySnapshotDBName, "agent activity snapshot store")
	agentActivityStoreDB := o.viewDB(storeDir, agentActivityDBName, "agent activity read db")
	agentChatStoreDB := o.viewDB(storeDir, agentChatDBName, "agent chat store db")
	agentRunnerEventStore := o.eventStore(eventsDir, agentRunnerDBName, "agent runner event store")
	agentRunnerSnapshotStore := o.snapshotStore(eventsDir, agentRunnerSnapshotDBName, "agent runner snapshot store")
	agentRunnerStoreDB := o.viewDB(storeDir, agentRunnerDBName, "agent runner store db")
	globalView := o.viewDB(stateDir, viewDBName, "global view")

	if o.err != nil {
		err = o.err
		return nil, err
	}

	c = &Container{
		crowbarHome:                home,
		reviewThreadES:             reviewThreadES,
		reviewThreadSS:             reviewThreadSS,
		workspaceEventStore:        workspaceEventStore,
		workspaceSnapshotStore:     workspaceSnapshotStore,
		workspaceStoreDB:           workspaceStoreDB,
		reviewThreadView:           reviewThreadView,
		agentChatEventStore:        agentChatEventStore,
		agentChatSnapshotStore:     agentChatSnapshotStore,
		agentChatStoreDB:           agentChatStoreDB,
		agentActivityEventStore:    agentActivityEventStore,
		agentActivitySnapshotStore: agentActivitySnapshotStore,
		agentActivityStoreDB:       agentActivityStoreDB,
		agentRunnerEventStore:      agentRunnerEventStore,
		agentRunnerSnapshotStore:   agentRunnerSnapshotStore,
		agentRunnerStoreDB:         agentRunnerStoreDB,
		globalView:                 globalView,
		lock:                       lock,
	}
	c.globalClosers = collectClosers(reviewThreadES, reviewThreadSS)
	return c, nil
}

// opens is a fail-fast opener for newLocked: it keeps the FIRST error and the
// rollback for everything opened before it.
//
// Fail-fast matters as much as the rollback. Once one open has failed the rest are
// no-ops, so the caller does not have to re-check between every line, and the
// container is never built from a half-open set.
type opens struct {
	err      error
	rollback []func() error
}

// unwind closes what was opened, newest first. Only called when newLocked is
// returning an error — on success the Container owns these and Close has them.
func (o *opens) unwind() {
	for i := len(o.rollback) - 1; i >= 0; i-- {
		_ = o.rollback[i]()
	}
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

// WorkspaceSS returns the workspace snapshot store at
// state/events/workspace_snapshots.db — the asynx v0.8 O(1) warm-read cache
// paired with WorkspaceES. It is the store the workspace-scoped read hot path
// (scopeWorkspaceToPath -> workspace.Get -> Reader.Load) now Gets by
// aggregateID instead of the old O(n) snapshot-stream scan.
func (c *Container) WorkspaceSS() asynxModels.SnapshotStore {
	return c.workspaceSnapshotStore
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

// AgentChatSS returns the agent-chat snapshot store at
// state/events/agent_chat_snapshots.db — the asynx v0.8 O(1) warm-read cache
// paired with AgentChatES.
func (c *Container) AgentChatSS() asynxModels.SnapshotStore {
	return c.agentChatSnapshotStore
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
// AgentActivityES returns the conversation-record event log at
// state/events/agent_activity.db.
func (c *Container) AgentActivityES() asynxModels.Store {
	return c.agentActivityEventStore
}

// AgentActivitySS returns the conversation-record snapshot store.
func (c *Container) AgentActivitySS() asynxModels.SnapshotStore {
	return c.agentActivitySnapshotStore
}

// AgentActivityReadDB returns the conversation-record read model at
// state/store/agent_activity.db. Unlike the other read models it holds ROWS, not
// one JSON blob per aggregate: turns, tool calls, subagents and interruptions are
// queried by name, by file and by time, which a blob cannot answer.
func (c *Container) AgentActivityReadDB() *gormdb.DB {
	return c.agentActivityStoreDB
}

func (c *Container) AgentRunnerES() asynxModels.Store {
	return c.agentRunnerEventStore
}

// AgentRunnerSS returns the agent-runner snapshot store at
// state/events/agent_runner_snapshots.db — the asynx v0.8 O(1) warm-read cache
// paired with AgentRunnerES.
func (c *Container) AgentRunnerSS() asynxModels.SnapshotStore {
	return c.agentRunnerSnapshotStore
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

	// Shutdown is a LIST, not a flow. Every entry is the same three steps — skip if
	// never built, close it, forget it — and the only thing that varies is which
	// handle and what to call it in the error. Written out inline that was twelve
	// identical branches; written as a list, adding a store is one line and cannot
	// forget to nil the field.
	closeEach(&errs, "workspace store db", &c.workspaceStoreDB, closeViewDB)
	closeEach(&errs, "review thread view", &c.reviewThreadView, closeViewDB)
	closeEach(&errs, "agent activity store db", &c.agentActivityStoreDB, closeViewDB)
	closeEach(&errs, "agent chat store db", &c.agentChatStoreDB, closeViewDB)
	closeEach(&errs, "agent runner store db", &c.agentRunnerStoreDB, closeViewDB)
	closeEach(&errs, "global view", &c.globalView, closeViewDB)

	closeEach(&errs, "workspace event store", &c.workspaceEventStore, closeEventStore)
	closeEach(&errs, "workspace snapshot store", &c.workspaceSnapshotStore, closeSnapshotStore)
	closeEach(&errs, "agent activity event store", &c.agentActivityEventStore, closeEventStore)
	closeEach(&errs, "agent activity snapshot store", &c.agentActivitySnapshotStore, closeSnapshotStore)
	closeEach(&errs, "agent chat event store", &c.agentChatEventStore, closeEventStore)
	closeEach(&errs, "agent chat snapshot store", &c.agentChatSnapshotStore, closeSnapshotStore)
	closeEach(&errs, "agent runner event store", &c.agentRunnerEventStore, closeEventStore)
	closeEach(&errs, "agent runner snapshot store", &c.agentRunnerSnapshotStore, closeSnapshotStore)

	for _, cl := range c.globalClosers {
		if err := cl.Close(); err != nil {
			errs = append(errs, fmt.Errorf("adapter: close global event store: %w", err))
		}
	}
	c.globalClosers = nil

	closeEach(&errs, "release state lock", &c.lock, (*instanceLock).Close)
	return errors.Join(errs...)
}

// closeEach closes one handle and clears it, collecting a named error.
//
// The nil check is on the INTERFACE/pointer value, so a handle that was never
// built is skipped — Close runs on partially-constructed containers, which is the
// whole reason a boot failure can still shut down cleanly. Clearing the field
// makes Close idempotent.
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
