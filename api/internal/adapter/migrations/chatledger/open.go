package chatledger

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"

	glebarez "github.com/glebarez/sqlite"
)

// openLegacy opens the database at path for reading only; a missing file is
// empty input. immutable=1 creates no -wal or -shm, but cannot see pages still
// in a WAL, so a non-empty one beside the file opens with mode=ro instead.
func openLegacy(path string) (*sql.DB, bool, error) {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, fmt.Errorf("chatledger: open %s: %w", path, err)
	}
	query := "immutable=1"
	if wal, err := os.Stat(path + "-wal"); err == nil && wal.Size() > 0 {
		query = "mode=ro"
	}
	dsn := url.URL{Scheme: "file", Path: path, RawQuery: query}
	db, err := sql.Open(glebarez.DriverName, dsn.String())
	if err != nil {
		return nil, false, fmt.Errorf("chatledger: open %s: %w", path, err)
	}
	return db, true, nil
}
