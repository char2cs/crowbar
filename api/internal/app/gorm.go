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
// Folders is typed as the wider ScopedStore because every folder read in a
// request path is repo-scoped: the sidebar wants one repo's rows, and a
// whole-table FindAll would grow with the install rather than with the request.
// AgentChatFolders is scoped for the same reason one level down: the Chats panel
// wants ONE workspace's folders, and every install has many workspaces.
type GORMStores struct {
	Projects                 store.Store[domain.Project, string]
	Repositories             store.ScopedStore[domain.Repository, string]
	Folders                  store.ScopedStore[domain.Folder, string]
	AgentChatFolders         store.ScopedStore[domain.ChatFolder, string]
	TerminalProfiles         store.Store[domain.TerminalProfile, string]
	TerminalSessions         store.Store[domain.TerminalSession, string]
	AgentProviderPreferences store.Store[domain.AgentProviderPreference, string]
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
	folders, err := storesqlite.NewFromDB[domain.Folder, string](db)
	if err != nil {
		return nil, fmt.Errorf("app: folder store: %w", err)
	}
	chatFolders, err := storesqlite.NewFromDB[domain.ChatFolder, string](db)
	if err != nil {
		return nil, fmt.Errorf("app: agent chat folder store: %w", err)
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
	return &GORMStores{
		Projects:                 projects,
		Repositories:             repos,
		Folders:                  folders,
		AgentChatFolders:         chatFolders,
		TerminalProfiles:         profiles,
		TerminalSessions:         sessions,
		AgentProviderPreferences: providerPrefs,
	}, nil
}
