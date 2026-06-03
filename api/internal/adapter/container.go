package adapter

import (
	"errors"
	"fmt"
	"path/filepath"

	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/core/paths"
)

// Container holds all adapter-layer storage backends opened by New.
// Callers must invoke Close during shutdown to release file handles and
// checkpoint WAL files.
type Container struct {
	Store        *gormdb.DB
	TaskES       asynxModels.Store
	AgentRunES   sqlite.EventStore
	KanbanItemES asynxModels.Store
	ThreadES     asynxModels.Store
	close        []func() error
}

// Close calls every registered closer in order, collecting any errors.
func (c *Container) Close() error {
	var errs []error
	for _, fn := range c.close {
		if err := fn(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type adapterOpts struct{ homeDir string }

// Option configures adapter.New.
type Option func(*adapterOpts)

// WithHomeDir overrides the home directory used for path resolution,
// bypassing the process-level HOME env var. Intended for test isolation.
func WithHomeDir(dir string) Option {
	return func(o *adapterOpts) { o.homeDir = dir }
}

// New constructs all adapter-layer storage backends:
//   - a GORM SQLite database at <store>/crowbar.db
//   - four Asynx SQLite event stores under <events>/
func New(opts ...Option) (*Container, error) {
	cfg := adapterOpts{}
	for _, o := range opts {
		o(&cfg)
	}

	// Resolve paths, creating directories on demand.
	var storePath, eventsPath string
	var err error
	if cfg.homeDir != "" {
		storePath, err = paths.StoreAt(cfg.homeDir)
		if err != nil {
			return nil, fmt.Errorf("adapter: store path: %w", err)
		}
		eventsPath, err = paths.EventsAt(cfg.homeDir)
	} else {
		storePath, err = paths.Store()
		if err != nil {
			return nil, fmt.Errorf("adapter: store path: %w", err)
		}
		eventsPath, err = paths.Events()
	}
	if err != nil {
		return nil, fmt.Errorf("adapter: events path: %w", err)
	}

	// Open GORM read-model database.
	db, err := sqlite.OpenDB(filepath.Join(storePath, "crowbar.db"))
	if err != nil {
		return nil, fmt.Errorf("adapter: crowbar.db: %w", err)
	}

	// Open four event stores — one SQLite file per aggregate type.
	taskES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "tasks.db"))
	if err != nil {
		return nil, fmt.Errorf("adapter: tasks event store: %w", err)
	}

	agentRunES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "agent_runs.db"))
	if err != nil {
		_ = taskES.Close()
		return nil, fmt.Errorf("adapter: agent_runs event store: %w", err)
	}

	kanbanItemES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "kanban_items.db"))
	if err != nil {
		_ = taskES.Close()
		_ = agentRunES.Close()
		return nil, fmt.Errorf("adapter: kanban_items event store: %w", err)
	}

	threadES, err := sqlite.NewEventStore(filepath.Join(eventsPath, "review_threads.db"))
	if err != nil {
		_ = taskES.Close()
		_ = agentRunES.Close()
		_ = kanbanItemES.Close()
		return nil, fmt.Errorf("adapter: review_threads event store: %w", err)
	}

	// Register close functions in reverse open order.
	closers := []func() error{
		func() error {
			sqlDB, err := db.DB()
			if err != nil {
				return err
			}
			return sqlDB.Close()
		},
		taskES.Close,
		agentRunES.Close,
		kanbanItemES.Close,
		threadES.Close,
	}

	return &Container{
		Store:        db,
		TaskES:       taskES,
		AgentRunES:   agentRunES,
		KanbanItemES: kanbanItemES,
		ThreadES:     threadES,
		close:        closers,
	}, nil
}
