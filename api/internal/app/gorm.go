package app

import (
	"fmt"

	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// GORMStores holds the plain-CRUD repositories backed by the shared GORM DB.
//
// Folders is wired here purely additively (2026-09-08
// sidebar-placement-unification, Task 2): nothing reads or writes it yet, and
// the sidebar's existing folder rows are still domain.Chat rows (Type ==
// ChatTypeFolder) until a later task in the same plan migrates them over. It
// is plain GORM, not asynx-backed like domain.Node/Chat, per the design
// spec's §2.2 reasoning: a folder rename is a single, low-stakes CRUD fact
// with no drag-time densify race to guard and no motivated undo case.
type GORMStores struct {
	Projects                 store.Store[domain.Project, string]
	Repositories             store.ScopedStore[domain.Repository, string]
	TerminalProfiles         store.Store[domain.TerminalProfile, string]
	TerminalSessions         store.Store[domain.TerminalSession, string]
	AgentProviderPreferences store.Store[domain.AgentProviderPreference, string]
	AgentPermissionDefault   store.Store[domain.AgentPermissionDefault, string]
	Folders                  store.ScopedStore[domain.Folder, string]
}

func newGORMStores(
	db *gormdb.DB,
) (*GORMStores, error) {
	projects, err := storesqlite.NewFromDB[domain.Project, string](db)
	if err != nil {
		return nil, fmt.Errorf("app: project store: %w", err)
	}
	repos, err := storesqlite.NewFromDB[domain.Repository, string](db)
	if err != nil {
		return nil, fmt.Errorf("app: repository store: %w", err)
	}
	profiles, err := storesqlite.NewFromDB[domain.TerminalProfile, string](db)
	if err != nil {
		return nil, fmt.Errorf("app: terminal profile store: %w", err)
	}
	sessions, err := storesqlite.NewFromDB[domain.TerminalSession, string](db)
	if err != nil {
		return nil, fmt.Errorf("app: terminal session store: %w", err)
	}
	providerPrefs, err := storesqlite.NewFromDB[domain.AgentProviderPreference, string](db)
	if err != nil {
		return nil, fmt.Errorf("app: agent provider preference store: %w", err)
	}
	permissionDefault, err := storesqlite.NewFromDB[domain.AgentPermissionDefault, string](db)
	if err != nil {
		return nil, fmt.Errorf("app: agent permission default store: %w", err)
	}
	folders, err := storesqlite.NewFromDB[domain.Folder, string](db)
	if err != nil {
		return nil, fmt.Errorf("app: folder store: %w", err)
	}
	return &GORMStores{
		Projects:                 projects,
		Repositories:             repos,
		TerminalProfiles:         profiles,
		TerminalSessions:         sessions,
		AgentProviderPreferences: providerPrefs,
		AgentPermissionDefault:   permissionDefault,
		Folders:                  folders,
	}, nil
}
