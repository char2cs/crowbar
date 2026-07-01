package app

import (
	"fmt"

	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// GORMStores holds the plain-CRUD repositories backed by the shared GORM DB.
type GORMStores struct {
	Projects         store.Store[domain.Project, string]
	Repositories     store.Store[domain.Repository, string]
	TerminalProfiles store.Store[domain.TerminalProfile, string]
	TerminalSessions store.Store[domain.TerminalSession, string]
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
	return &GORMStores{
		Projects:         projects,
		Repositories:     repos,
		TerminalProfiles: profiles,
		TerminalSessions: sessions,
	}, nil
}
