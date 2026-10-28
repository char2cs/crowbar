package sqlite

import (
	"bytes"
	"context"
	"fmt"
	"testing"
)

// prodSnapshotBlobs mirrors the production aggregate the perf audit measured:
// one workspace had 4013 accumulated snapshot blobs totalling ~2.7MB (~672
// bytes each).
const (
	prodSnapshotBlobs = 4013
	prodSnapshotBytes = 672
)

// BenchmarkSnapshotRead_New_Get reproduces the asynx v0.8.0 read hot path:
// SnapshotStore.Get(id) reads the single upserted row keyed by aggregateID.
// The store is seeded by upserting the same prodSnapshotBlobs versions, which
// (unlike the old append) collapse to ONE row — the O(1) read the upgrade
// delivers.
func BenchmarkSnapshotRead_New_Get(b *testing.B) {
	s, err := NewSnapshotStore(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	id := "ws-heavy"
	blob := bytes.Repeat([]byte("x"), prodSnapshotBytes)
	for v := 1; v <= prodSnapshotBlobs; v++ {
		if err := s.Put(ctx, id, int64(v), blob); err != nil {
			b.Fatal(err)
		}
	}
	if n := rowCountBench(b, s, id); n != 1 {
		b.Fatalf("upsert must collapse to one row, got %d", n)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, found, err := s.Get(ctx, id)
		if err != nil {
			b.Fatal(err)
		}
		if !found {
			b.Fatal("snapshot must be found")
		}
	}
}

func rowCountBench(b *testing.B, s interface{}, aggregateID string) int64 {
	b.Helper()
	impl, ok := s.(*snapshotStore)
	if !ok {
		b.Fatalf("expected *snapshotStore, got %T", s)
	}
	var n int64
	if err := impl.db.
		Model(&snapshotEntry{}).
		Where("aggregate_id = ?", aggregateID).
		Count(&n).Error; err != nil {
		b.Fatal(fmt.Errorf("count: %w", err))
	}
	return n
}
