// Package sqlite provides a GORM-backed Store[T, K] implementation.
package sqlite

import (
	"context"
	"errors"
	"fmt"

	glebarez "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
)

type gormStore[T any, K comparable] struct {
	db    *gorm.DB
	pkCol string
}

// New opens (or creates) a SQLite-backed Store[T, K] at path, auto-migrating T.
func New[T any, K comparable](
	path string,
) (store.Store[T, K], error) {
	db, err := OpenDB(path)
	if err != nil {
		return nil, err
	}
	return NewFromDB[T, K](db)
}

// OpenDB opens (or creates) a single-connection SQLite database at path.
// WAL journal mode and a 5-second busy timeout are enabled so that a second
// opener (e.g. in crash-recovery tests) does not get SQLITE_BUSY on DDL. Used
// for the per-entity workspace databases, which are effectively single-tenant
// (one workspace, one or two open tabs at most).
func OpenDB(
	path string,
) (*gorm.DB, error) {
	return OpenDBWithPool(path, 1)
}

// OpenDBWithPool opens (or creates) a SQLite database at path with WAL journal
// mode, a 5-second busy timeout, and up to maxOpenConns open connections. WAL
// mode allows one writer plus many concurrent readers at the SQLite level;
// maxOpenConns controls how many of those concurrent readers the Go
// connection pool actually allows through at once — use a value greater than
// 1 for a database that serves concurrent read-heavy traffic (the global
// view.db), and 1 (via OpenDB) for a single-tenant per-entity database.
func OpenDBWithPool(
	path string,
	maxOpenConns int,
) (*gorm.DB, error) {
	db, err := gorm.Open(glebarez.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("sqlite: db: %w", err)
	}
	sqlDB.SetMaxOpenConns(maxOpenConns)
	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		return nil, fmt.Errorf("sqlite: journal_mode: %w", err)
	}
	if err := db.Exec("PRAGMA busy_timeout=5000").Error; err != nil {
		return nil, fmt.Errorf("sqlite: busy_timeout: %w", err)
	}
	return db, nil
}

// NewFromDB builds a Store[T, K] over an already-open DB, auto-migrating T.
func NewFromDB[T any, K comparable](
	db *gorm.DB,
) (store.Store[T, K], error) {
	var zero T
	if err := db.AutoMigrate(&zero); err != nil {
		return nil, fmt.Errorf("sqlite: migrate: %w", err)
	}
	pkCol, err := primaryKeyColumn[T](db)
	if err != nil {
		return nil, fmt.Errorf("sqlite: schema: %w", err)
	}
	return &gormStore[T, K]{db: db, pkCol: pkCol}, nil
}

func primaryKeyColumn[T any](
	db *gorm.DB,
) (string, error) {
	var zero T
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(&zero); err != nil {
		return "", err
	}
	namer := schema.NamingStrategy{}
	for _, field := range stmt.Schema.Fields {
		if field.PrimaryKey {
			return namer.ColumnName("", field.Name), nil
		}
	}
	return "", fmt.Errorf("no primary key field found")
}

func (s *gormStore[T, K]) Save(
	ctx context.Context,
	item T,
) error {
	return s.db.WithContext(ctx).Save(&item).Error
}

func (s *gormStore[T, K]) Delete(
	ctx context.Context,
	id K,
) error {
	var zero T
	return s.db.WithContext(ctx).Where(s.pkCol+" = ?", id).Delete(&zero).Error
}

func (s *gormStore[T, K]) FindByKey(
	ctx context.Context,
	id K,
) (*T, error) {
	var item T
	err := s.db.WithContext(ctx).Where(s.pkCol+" = ?", id).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *gormStore[T, K]) FindAll(
	ctx context.Context,
) ([]T, error) {
	var items []T
	if err := s.db.WithContext(ctx).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
