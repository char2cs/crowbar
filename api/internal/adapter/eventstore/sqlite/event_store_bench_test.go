package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/char2cs/asynx/models"
)

func benchAppend(b *testing.B, s models.Store, version int64) {
	b.Helper()
	ctx := context.Background()
	if err := s.Atomically(ctx, func(tx models.Tx) error {
		return tx.Append(ctx, "agg-bench", version, []byte("payload"))
	}); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkEventStore_AppendOnDisk measures an append against a real file, where
// the synchronous pragma decides whether every commit pays an fsync.
func BenchmarkEventStore_AppendOnDisk(b *testing.B) {
	s, err := NewEventStore(filepath.Join(b.TempDir(), "events.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchAppend(b, s, int64(i+1))
	}
}

func BenchmarkEventStore_ReadFrom(b *testing.B) {
	s, err := NewEventStore(filepath.Join(b.TempDir(), "events.db"))
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		benchAppend(b, s, int64(i+1))
	}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.ReadFrom(ctx, "agg-bench", 1); err != nil {
			b.Fatal(err)
		}
	}
}
