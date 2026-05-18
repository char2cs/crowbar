package sqlite_test

import (
	"path/filepath"
	"testing"

	"github.com/rabbytesoftware/crowbar/api/internal/adapter/store/sqlite"
)

func TestStoreOpenAndMigrate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	store, err := sqlite.New(path)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	defer store.Close()

	if store.DB() == nil {
		t.Fatal("expected non-nil DB")
	}
}
