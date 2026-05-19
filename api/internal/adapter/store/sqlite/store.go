package sqlite

import (
	"github.com/glebarez/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Store struct {
	db *gorm.DB
}

func New(path string) (*Store, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(
		&domain.Project{},
		&domain.Workspace{},
	); err != nil {
		return nil, err
	}

	return &Store{db: db}, nil
}

func (s *Store) DB() *gorm.DB { return s.db }

func (s *Store) Close() error {
	sql, err := s.db.DB()
	if err != nil {
		return err
	}
	return sql.Close()
}
