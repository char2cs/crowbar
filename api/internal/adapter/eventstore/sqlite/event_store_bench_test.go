package sqlite

import (
	"context"
	"testing"
)

func BenchmarkEventStore_Append(b *testing.B) {
	s, err := NewEventStore(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.Append(ctx, "agg-bench", int64(i+1), []byte("payload"))
	}
}

func BenchmarkEventStore_ReadFrom(b *testing.B) {
	s, err := NewEventStore(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 1000; i++ {
		_ = s.Append(ctx, "agg-bench", int64(i+1), []byte("payload"))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.ReadFrom(ctx, "agg-bench", 1)
	}
}
