package chatledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"

	glebarez "github.com/glebarez/sqlite"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
)

// openLegacy opens the database at path for reading only; a missing file is
// empty input.
func openLegacy(path string) (*sql.DB, bool, error) {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, fmt.Errorf("chatledger: open %s: %w", path, err)
	}
	db, err := sql.Open(glebarez.DriverName, storesqlite.ReadOnlyDSN(path))
	if err != nil {
		return nil, false, fmt.Errorf("chatledger: open %s: %w", path, err)
	}
	return db, true, nil
}

// tableColumns is the columns table has; none when the database holds no such table.
func tableColumns(ctx context.Context, db *sql.DB, table string) (_ map[string]bool, err error) {
	rows, err := db.QueryContext(ctx, "SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return nil, fmt.Errorf("chatledger: %s: %w", table, err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	has := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("chatledger: %s: %w", table, err)
		}
		has[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chatledger: %s: %w", table, err)
	}
	return has, nil
}

// queryLegacy runs the query built for the columns table has over it in the
// database at path, handing each row to scan. A missing database or table is no rows.
func queryLegacy(
	ctx context.Context, path, table string, scan func(*sql.Rows) error, query func(has map[string]bool) string, args ...any,
) (err error) {
	db, ok, err := openLegacy(path)
	if err != nil || !ok {
		return err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	has, err := tableColumns(ctx, db, table)
	if err != nil || len(has) == 0 {
		return err
	}
	rows, err := db.QueryContext(ctx, query(has), args...)
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
