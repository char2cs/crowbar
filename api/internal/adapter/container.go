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

const (
	viewDBName = "view.db"
)

const (
	workspaceDBName    = "workspace.db"
	reviewThreadDBName = "review_thread.db"
	agentChatDBName    = "agent_chat.db"
	treeDBName         = "tree.db"
)

const (
	workspaceSnapshotDBName    = "workspace_snapshots.db"
	reviewThreadSnapshotDBName = "review_thread_snapshots.db"
	agentChatSnapshotDBName    = "agent_chat_snapshots.db"
	treeSnapshotDBName         = "tree_snapshots.db"
)

type Container struct {
	home home.Home

	reviewThreadES asynxModels.Store
	reviewThreadSS asynxModels.SnapshotStore

	workspaceEventStore    asynxModels.Store
	workspaceSnapshotStore asynxModels.SnapshotStore
	workspaceStoreDB       *gormdb.DB
	reviewThreadView       *gormdb.DB
	agentChatEventStore    asynxModels.Store
	agentChatSnapshotStore asynxModels.SnapshotStore
	agentChatStoreDB       *gormdb.DB
	treeEventStore         asynxModels.Store
	treeSnapshotStore      asynxModels.SnapshotStore
	chatLedgerDB           *gormdb.DB

	globalView *gormdb.DB

	globalClosers []io.Closer
}

type adapterOpts struct {
	homeDir string
}

type Option func(*adapterOpts)

func WithHomeDir(
	dir string,
) Option {
	return func(o *adapterOpts) {
		o.homeDir = dir
	}
}

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
	for _, dir := range []string{h.Root(), h.State(), h.Events(), h.Store()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("adapter: create %s: %w", dir, err)
		}
		if dir == h.Root() {
			continue
		}
		if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // G302: a directory needs its search bit; 0700 is owner-only
			return nil, fmt.Errorf("adapter: restrict %s: %w", dir, err)
		}
	}

	return openContainer(h)
}

func openContainer(
	h home.Home,
) (c *Container, err error) {
	eventsDir, storeDir := h.Events(), h.Store()

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

type opens struct {
	err      error
	rollback []func() error
}

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

func (c *Container) ReviewThreadES() asynxModels.Store {
	return c.reviewThreadES
}

func (c *Container) ReviewThreadSS() asynxModels.SnapshotStore {
	return c.reviewThreadSS
}

func (c *Container) ReviewThreadView() *gormdb.DB {
	return c.reviewThreadView
}

func (c *Container) WorkspaceES() asynxModels.Store {
	return c.workspaceEventStore
}

func (c *Container) WorkspaceSS() asynxModels.SnapshotStore {
	return c.workspaceSnapshotStore
}

func (c *Container) WorkspaceView() *gormdb.DB {
	return c.workspaceStoreDB
}

func (c *Container) AgentChatES() asynxModels.Store {
	return c.agentChatEventStore
}

func (c *Container) AgentChatSS() asynxModels.SnapshotStore {
	return c.agentChatSnapshotStore
}

func (c *Container) AgentChatReadDB() *gormdb.DB {
	return c.agentChatStoreDB
}

func (c *Container) TreeES() asynxModels.Store {
	return c.treeEventStore
}

func (c *Container) TreeSS() asynxModels.SnapshotStore {
	return c.treeSnapshotStore
}

func (c *Container) GlobalView() *gormdb.DB {
	return c.globalView
}

func (c *Container) Home() home.Home {
	return c.home
}

func (c *Container) Close() error {
	var errs []error

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
