package adapter

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/core/paths"
)

// Container holds the persistence layer: one event store per Asynx aggregate and
// the shared GORM database.
type Container struct {
	WorkspaceES    asynxModels.Store
	ChatES         asynxModels.Store
	ReviewThreadES asynxModels.Store
	DB             *gormdb.DB
	closers        []io.Closer
	lock           *instanceLock
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

// New constructs all event stores and the GORM database.
func New(
	opts ...Option,
) (*Container, error) {
	cfg := adapterOpts{}
	for _, o := range opts {
		o(&cfg)
	}

	statePath, err := resolveStateDir(cfg.homeDir)
	if err != nil {
		return nil, err
	}

	lock, err := acquireStateLock(statePath)
	if err != nil {
		return nil, err
	}

	container, err := newLocked(cfg.homeDir, lock)
	if err != nil {
		_ = lock.Close()
		return nil, err
	}
	return container, nil
}

func newLocked(
	homeDir string,
	lock *instanceLock,
) (*Container, error) {
	eventsPath, storePath, err := resolveDirs(homeDir)
	if err != nil {
		return nil, err
	}

	stores, closers, err := openEventStores(eventsPath)
	if err != nil {
		return nil, err
	}

	db, err := storesqlite.OpenDB(filepath.Join(storePath, "crowbar.db"))
	if err != nil {
		for _, cl := range closers {
			_ = cl.Close()
		}
		return nil, fmt.Errorf("adapter: open db: %w", err)
	}

	return &Container{
		WorkspaceES:    stores[0],
		ChatES:         stores[1],
		ReviewThreadES: stores[2],
		DB:             db,
		closers:        closers,
		lock:           lock,
	}, nil
}

func resolveStateDir(
	homeDir string,
) (string, error) {
	if homeDir == "" {
		statePath, err := paths.State()
		if err != nil {
			return "", fmt.Errorf("adapter: state dir: %w", err)
		}
		return statePath, nil
	}
	statePath, err := paths.StateAt(homeDir)
	if err != nil {
		return "", fmt.Errorf("adapter: state dir: %w", err)
	}
	return statePath, nil
}

func resolveDirs(
	homeDir string,
) (string, string, error) {
	if homeDir == "" {
		return resolveDefaultDirs()
	}
	return resolveHomeDirs(homeDir)
}

func resolveDefaultDirs() (string, string, error) {
	eventsPath, err := paths.Events()
	if err != nil {
		return "", "", fmt.Errorf("adapter: events: %w", err)
	}
	storePath, err := paths.Store()
	if err != nil {
		return "", "", fmt.Errorf("adapter: store: %w", err)
	}
	return eventsPath, storePath, nil
}

func resolveHomeDirs(
	homeDir string,
) (string, string, error) {
	eventsPath, err := paths.EventsAt(homeDir)
	if err != nil {
		return "", "", fmt.Errorf("adapter: events: %w", err)
	}
	storePath, err := paths.StoreAt(homeDir)
	if err != nil {
		return "", "", fmt.Errorf("adapter: store: %w", err)
	}
	return eventsPath, storePath, nil
}

func openEventStores(
	eventsPath string,
) ([]asynxModels.Store, []io.Closer, error) {
	names := []string{"workspace.db", "chat.db", "review_thread.db"}
	stores := make([]asynxModels.Store, 0, len(names))
	closers := make([]io.Closer, 0, len(names))
	for _, name := range names {
		es, err := eventsqlite.NewEventStore(filepath.Join(eventsPath, name))
		if err != nil {
			return nil, nil, fmt.Errorf("adapter: event store %s: %w", name, err)
		}
		stores = append(stores, es)
		if cl, ok := es.(io.Closer); ok {
			closers = append(closers, cl)
		}
	}
	return stores, closers, nil
}

// Close checkpoints and closes every event store and the shared GORM database.
func (c *Container) Close() error {
	var errs []error
	for _, cl := range c.closers {
		if err := cl.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if c.DB != nil {
		sqlDB, err := c.DB.DB()
		if err != nil {
			errs = append(errs, fmt.Errorf("adapter: close db handle: %w", err))
		} else if err := sqlDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("adapter: close db: %w", err))
		}
	}
	if c.lock != nil {
		if err := c.lock.Close(); err != nil {
			errs = append(errs, fmt.Errorf("adapter: release state lock: %w", err))
		}
		c.lock = nil
	}
	return errors.Join(errs...)
}
