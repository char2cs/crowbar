package chatledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"

	glebarez "github.com/glebarez/sqlite"
)

// openLegacy opens the database at path for reading only; a missing file is
// empty input. immutable=1 creates no -wal or -shm but cannot see a WAL, so a
// database still written, or with a non-empty WAL beside it, opens mode=ro.
func openLegacy(path string, written bool) (*sql.DB, bool, error) {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, fmt.Errorf("chatledger: open %s: %w", path, err)
	}
	query := "immutable=1"
	if wal, err := os.Stat(path + "-wal"); written || err == nil && wal.Size() > 0 {
		query = "mode=ro"
	}
	dsn := url.URL{Scheme: "file", Path: path, RawQuery: query}
	db, err := sql.Open(glebarez.DriverName, dsn.String())
	if err != nil {
		return nil, false, fmt.Errorf("chatledger: open %s: %w", path, err)
	}
	return db, true, nil
}

// queryLegacy runs query over table in the database at path, handing each row
// to scan. A missing database or table is no rows.
func queryLegacy(
	ctx context.Context, path, table string, scan func(*sql.Rows) error, query string, args ...any,
) (err error) {
	db, ok, err := openLegacy(path, filepath.Base(path) == ActivityDB)
	if err != nil || !ok {
		return err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	var n int
	if err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&n); err != nil {
		return fmt.Errorf("chatledger: %s: %w", table, err)
	}
	if n == 0 {
		return nil
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("chatledger: %s: %w", table, err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return fmt.Errorf("chatledger: %s: %w", table, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("chatledger: %s: %w", table, err)
	}
	return nil
}
