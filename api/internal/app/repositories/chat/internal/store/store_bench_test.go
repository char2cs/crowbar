package store

import (
	"context"
	"fmt"
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
	c := domain.Chat{ID: "c1", WsID: "w1", CreatedAt: time.Unix(1, 0)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := st.Save(ctx, c); err != nil {
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
		c := domain.Chat{ID: fmt.Sprintf("c%d", i), WsID: "w1"}
		_ = st.Save(ctx, c)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := st.FindAll(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
