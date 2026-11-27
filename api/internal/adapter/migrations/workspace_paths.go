package migrations

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/adapter/migrations/chatledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
)

const retiredTable = "workspace_paths"

func PreOpenImpls() migrate.Impls {
	return migrate.Impls{
		StepWorkspacePathsTable: Adopting(StepWorkspacePathsTable, migrate.Impl{
			Apply:  dropWorkspacePaths,
			Verify: verifyWorkspacePaths,
		}),
		StepChatLedger:      chatledger.Impl(),
		StepChatAttachments: {Apply: moveChatAttachments},
		StepLedgerLayout:    {Apply: func(context.Context, migrate.Env, *migrate.Checkpoints) (migrate.Counts, error) { return nil, nil }},
		StepRetireLegacy:    RetireImpl(),
		StepOwnerOnly:       {Apply: narrowOwnerOnly},
		StepWorktreeReach:   {Apply: reachWorktrees},
	}
}

func planWorkspacePaths(
	_ context.Context,
	env migrate.Env,
) (migrate.Plan, error) {
	exists, err := viewDBExists(env.Home)
	if err != nil || !exists {
		return migrate.Plan{}, err
	}
	return migrate.Plan{
		Actions: []migrate.Action{{
			Kind: migrate.Convert, Subject: "state/view.db: drop table " + retiredTable, Count: migrate.Unknown,
		}},
		RewritesState: true,
	}, nil
}

func dropWorkspacePaths(
	ctx context.Context,
	env migrate.Env,
	_ *migrate.Checkpoints,
) (migrate.Counts, error) {
	return nil, withViewDB(env.Home, func(db *gormdb.DB) error {
		return dropRetiredTable(ctx, db, retiredTable)
	})
}

func verifyWorkspacePaths(
	ctx context.Context,
	env migrate.Env,
) error {
	return withViewDB(env.Home, func(db *gormdb.DB) error {
		if db.WithContext(ctx).Migrator().HasTable(retiredTable) {
			return fmt.Errorf("migrations: view.db still holds table %s", retiredTable)
		}
		return nil
	})
}

func dropRetiredTable(
	ctx context.Context,
	db *gormdb.DB,
	table string,
) error {
	if err := db.WithContext(ctx).Migrator().DropTable(table); err != nil {
		return fmt.Errorf("migrations: drop table %s: %w", table, err)
	}
	return nil
}

func withViewDB(
	h home.Home,
	fn func(db *gormdb.DB) error,
) error {
	exists, err := viewDBExists(h)
	if err != nil || !exists {
		return err
	}
	db, err := storesqlite.OpenDB(viewDBPath(h))
	if err != nil {
		return fmt.Errorf("migrations: open view.db: %w", err)
	}
	return errors.Join(fn(db), closeDB(db))
}

func viewDBExists(
	h home.Home,
) (bool, error) {
	_, err := os.Stat(viewDBPath(h))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("migrations: view.db: %w", err)
	}
}

func viewDBPath(
	h home.Home,
) string {
	return filepath.Join(h.State(), "view.db")
}

func closeDB(
	db *gormdb.DB,
) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("migrations: view.db handle: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("migrations: close view.db: %w", err)
	}
	return nil
}
