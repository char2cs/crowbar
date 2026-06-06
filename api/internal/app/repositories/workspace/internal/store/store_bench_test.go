package store

import (
	"context"
	"testing"
	"time"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func BenchmarkStorage_Save(b *testing.B) {
	db, err := storesqlite.OpenDB(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	st, err := newStorageStore(db)
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	ws := domain.Workspace{ID: "w1", RepoID: "r1", ProjectID: "p1", CreatedAt: time.Unix(1, 0)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := st.Save(ctx, ws); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStorage_FindAll(b *testing.B) {
	db, err := storesqlite.OpenDB(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	st, err := newStorageStore(db)
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		ws := domain.Workspace{ID: string(rune('a'+i%26)) + string(rune('0'+i/26)), RepoID: "r1", ProjectID: "p1"}
		_ = st.Save(ctx, ws)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := st.FindAll(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
