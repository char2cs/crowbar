package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
)

func TestOpenDB_HappyPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.OpenDB(path)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if db == nil {
		t.Fatal("expected non-nil db")
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("expected no error getting sql.DB, got: %v", err)
	}
	_ = sqlDB.Close()
}

func TestNewEventStore_HappyPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.db")
	store, err := sqlite.NewEventStore(path)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	defer store.Close()
}

func TestNewEventStore_Close(t *testing.T) {
	path := filepath.Join(t.TempDir(), "close.db")
	store, err := sqlite.NewEventStore(path)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("expected no error on close, got: %v", err)
	}
}

func TestEventStore_Append_And_ReadFrom(t *testing.T) {
	path := filepath.Join(t.TempDir(), "append.db")
	store, err := sqlite.NewEventStore(path)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.Append(ctx, "agg1", 0, []byte("first")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := store.Append(ctx, "agg1", 1, []byte("second")); err != nil {
		t.Fatalf("Append: %v", err)
	}

	rows, err := store.ReadFrom(ctx, "agg1", 0)
	if err != nil {
		t.Fatalf("ReadFrom: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if string(rows[0]) != "first" {
		t.Fatalf("expected 'first', got %q", string(rows[0]))
	}
}

func TestEventStore_Append_VersionConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conflict.db")
	store, err := sqlite.NewEventStore(path)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.Append(ctx, "agg2", 0, []byte("v0")); err != nil {
		t.Fatalf("first Append: %v", err)
	}
	if err := store.Append(ctx, "agg2", 0, []byte("dup")); err == nil {
		t.Fatal("expected error on duplicate version, got nil")
	}
}

func TestEventStore_ReadRange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "range.db")
	store, err := sqlite.NewEventStore(path)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	for i := int64(0); i < 5; i++ {
		if err := store.Append(ctx, "agg3", i, []byte("data")); err != nil {
			t.Fatalf("Append v%d: %v", i, err)
		}
	}

	rows, err := store.ReadRange(ctx, "agg3", 0, 3)
	if err != nil {
		t.Fatalf("ReadRange: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
}

func TestEventStore_Count(t *testing.T) {
	path := filepath.Join(t.TempDir(), "count.db")
	store, err := sqlite.NewEventStore(path)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	for i := int64(0); i < 4; i++ {
		if err := store.Append(ctx, "agg4", i, []byte("x")); err != nil {
			t.Fatalf("Append v%d: %v", i, err)
		}
	}

	count, err := store.Count(ctx, "agg4", 0)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 4 {
		t.Fatalf("expected 4, got %d", count)
	}

	count2, err := store.Count(ctx, "agg4", 2)
	if err != nil {
		t.Fatalf("Count(from=2): %v", err)
	}
	if count2 != 2 {
		t.Fatalf("expected 2, got %d", count2)
	}
}

func TestEventStore_Delete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delete.db")
	store, err := sqlite.NewEventStore(path)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.Append(ctx, "agg5", 0, []byte("to delete")); err != nil {
		t.Fatalf("Append: %v", err)
	}

	if err := store.Delete(ctx, "agg5"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	rows, err := store.ReadFrom(ctx, "agg5", 0)
	if err != nil {
		t.Fatalf("ReadFrom after delete: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows after delete, got %d", len(rows))
	}
}

func TestEventStore_ReadFrom_Empty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.db")
	store, err := sqlite.NewEventStore(path)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer store.Close()

	rows, err := store.ReadFrom(context.Background(), "nonexistent", 0)
	if err != nil {
		t.Fatalf("ReadFrom empty: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

func TestEventStore_ReadRange_Empty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "emptyrange.db")
	store, err := sqlite.NewEventStore(path)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer store.Close()

	rows, err := store.ReadRange(context.Background(), "nonexistent", 0, 10)
	if err != nil {
		t.Fatalf("ReadRange empty: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

func TestStoreClose_HappyPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "close.db")
	store, err := sqlite.New(path)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("expected no error on close, got: %v", err)
	}
}

func TestEventStore_ReadFrom_AfterClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "afterclose-rf.db")
	store, err := sqlite.NewEventStore(path)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	_ = store.Close()
	_, err = store.ReadFrom(context.Background(), "agg", 0)
	if err == nil {
		t.Fatal("expected error reading from closed store, got nil")
	}
}

func TestEventStore_ReadRange_AfterClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "afterclose-rr.db")
	store, err := sqlite.NewEventStore(path)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	_ = store.Close()
	_, err = store.ReadRange(context.Background(), "agg", 0, 10)
	if err == nil {
		t.Fatal("expected error on closed store, got nil")
	}
}

func TestEventStore_Count_AfterClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "afterclose-c.db")
	store, err := sqlite.NewEventStore(path)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	_ = store.Close()
	_, err = store.Count(context.Background(), "agg", 0)
	if err == nil {
		t.Fatal("expected error on closed store, got nil")
	}
}

func TestEventStore_Delete_AfterClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "afterclose-d.db")
	store, err := sqlite.NewEventStore(path)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	_ = store.Close()
	err = store.Delete(context.Background(), "agg")
	if err == nil {
		t.Fatal("expected error on closed store, got nil")
	}
}

func TestNewEventStore_InvalidPath(t *testing.T) {
	_, err := sqlite.NewEventStore("/dev/null/nonexistent/test.db")
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
}

func TestOpenDB_InvalidPath(t *testing.T) {
	_, err := sqlite.OpenDB("/dev/null/nonexistent/test.db")
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
}

func TestNew_InvalidPath(t *testing.T) {
	_, err := sqlite.New("/dev/null/nonexistent/test.db")
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
}
