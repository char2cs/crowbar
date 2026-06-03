package sqlite

import (
	"database/sql"

	"github.com/char2cs/crowbar/api/internal/domain"
	glebsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Store struct {
	db    *gorm.DB
	sqlDB *sql.DB
}

func New(path string) (*Store, error) {
	db, err := gorm.Open(glebsqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(
		&domain.Project{},
	); err != nil {
		return nil, err
	}

	sqlDB, _ := db.DB()
	return &Store{db: db, sqlDB: sqlDB}, nil
}

func (s *Store) DB() *gorm.DB { return s.db }

func (s *Store) Close() error {
	return s.sqlDB.Close()
}
