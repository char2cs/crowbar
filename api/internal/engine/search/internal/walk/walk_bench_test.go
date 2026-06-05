package walk_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/search/internal/walk"
)

// BenchmarkWalk_500Files benchmarks walking a directory tree with 500 files.
func BenchmarkWalk_500Files(b *testing.B) {
	dir := b.TempDir()

	// Create 500 files spread across 10 subdirectories.
	for i := 0; i < 500; i++ {
		sub := filepath.Join(dir, fmt.Sprintf("pkg%d", i%10))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			b.Fatal(err)
		}
		p := filepath.Join(sub, fmt.Sprintf("file%d.go", i))
		if err := os.WriteFile(p, []byte("package main\n"), 0o600); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ch, err := walk.Walk(context.Background(), dir, nopIgnorer{}, nil, nil)
		if err != nil {
			b.Fatal(err)
		}
		for range ch {
		}
	}
}
