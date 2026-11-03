// Package sqlite provides a GORM-backed Store[T, K] implementation.
package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

// New opens (or creates) a SQLite-backed ScopedStore[T, K] at path,
// auto-migrating T. Like NewFromDB it returns the wider ScopedStore, which
// satisfies Store everywhere the narrow surface is all that is wanted.
func New[T any, K comparable](
	path string,
) (store.ScopedStore[T, K], error) {
	db, err := OpenDB(path)
	if err != nil {
		return nil, err
	}
	return NewFromDB[T, K](db)
}

// DSN carries per-connection pragmas, since the driver applies DSN pragmas on every pooled connection.
// synchronous=NORMAL under WAL survives app crashes (only power loss can drop the last commits) and
// avoids FULL's fsync per append, paid several times per mutation.
func DSN(path string) string {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + "_pragma=synchronous(NORMAL)"
}

// readPoolConns sizes read-model/view pools so one long read cannot head-of-line-block the others
// under WAL; event logs stay single-writer.
const readPoolConns = 4

// OpenDB opens a single-connection SQLite DB with WAL and a 5s busy timeout so a second opener gets no
// SQLITE_BUSY on DDL. Used for per-entity workspace DBs, effectively single-tenant.
func OpenDB(
	path string,
) (*gorm.DB, error) {
	return openWithMaxConns(path, 1)
}

// OpenReadPoolDB opens a multi-connection read pool for read-model/view DBs. WAL lives in the header
// and busy_timeout is applied per connection, so every pooled connection inherits them.
func OpenReadPoolDB(
	path string,
) (*gorm.DB, error) {
	return openWithMaxConns(path, readPoolConns)
}

func openWithMaxConns(
	path string,
	maxConns int,
) (*gorm.DB, error) {
	db, err := gorm.Open(glebarez.Open(DSN(path)), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("sqlite: db: %w", err)
	}
	sqlDB.SetMaxOpenConns(maxConns)
	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		return nil, fmt.Errorf("sqlite: journal_mode: %w", err)
	}
	if err := db.Exec("PRAGMA busy_timeout=5000").Error; err != nil {
		return nil, fmt.Errorf("sqlite: busy_timeout: %w", err)
	}
	return db, nil
}

// NewFromDB builds a ScopedStore over an open DB, auto-migrating T; returning the wider type gives
// FindWhere callers no type assertion.
func NewFromDB[T any, K comparable](
	db *gorm.DB,
) (store.ScopedStore[T, K], error) {
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

// FindWhere implements store.ScopedStore, pushing the equality match down as a GORM struct condition;
// zero fields are ignored (see the interface doc).
func (s *gormStore[T, K]) FindWhere(
	ctx context.Context,
	match T,
) ([]T, error) {
	var items []T
	if err := s.db.WithContext(ctx).Where(&match).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
