package migrate

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

// tickingClock advances one second per call, so every record and run id of a
// test is distinct and ordered.
func tickingClock() func() time.Time {
	at := ledgerEpoch
	return func() time.Time {
		at = at.Add(time.Second)
		return at
	}
}

// bench is a fixture home with a catalog. Each begin is another boot of the
// same home, on one clock.
type bench struct {
	tb    testing.TB
	home  home.Home
	cat   *Catalog
	clock func() time.Time
}

func newBench(
	tb testing.TB,
	steps ...Step,
) *bench {
	tb.Helper()
	cat, err := NewCatalog(steps...)
	require.NoError(tb, err)
	return &bench{tb: tb, home: testutil.FixtureHome(tb), cat: cat, clock: tickingClock()}
}

// begin opens a session that is closed with the test; closing it earlier is fine.
func (b *bench) begin() *Session {
	b.tb.Helper()
	s, err := Begin(context.Background(), b.home, b.cat, Options{Version: "v-test", Clock: b.clock})
	require.NoError(b.tb, err)
	b.tb.Cleanup(func() {
		require.NoError(b.tb, s.Close())
	})
	return s
}

// records is what the ledger holds on disk now.
func (b *bench) records() []record {
	b.tb.Helper()
	view, err := readLedger(filepath.Join(b.home.MigrateDir(), ledgerName))
	require.NoError(b.tb, err)
	return view.records
}

func (b *bench) kinds() []string {
	b.tb.Helper()
	return kindsOf(b.records())
}

func (b *bench) layout() int {
	b.tb.Helper()
	layout, err := readLayout(b.home)
	require.NoError(b.tb, err)
	return layout
}

func kindsOf(
	recs []record,
) []string {
	out := make([]string, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.Kind)
	}
	return out
}

func ofKind(
	recs []record,
	kind string,
) []record {
	var out []record
	for _, r := range recs {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}

// impl is an Impl whose Apply is fn and which has nothing to verify.
func impl(
	fn func(cp *Checkpoints) (Counts, error),
) Impl {
	return Impl{Apply: func(_ context.Context, _ Env, cp *Checkpoints) (Counts, error) {
		return fn(cp)
	}}
}

// healthy gives each step number an Impl that does nothing.
func healthy(
	numbers ...int,
) Impls {
	impls := Impls{}
	for _, n := range numbers {
		impls[n] = impl(func(*Checkpoints) (Counts, error) {
			return nil, nil
		})
	}
	return impls
}

// tracing gives each step number an Impl that appends its number to order.
func tracing(
	order *[]int,
	numbers ...int,
) Impls {
	impls := Impls{}
	for _, n := range numbers {
		impls[n] = impl(func(*Checkpoints) (Counts, error) {
			*order = append(*order, n)
			return nil, nil
		})
	}
	return impls
}

func halted(
	tb testing.TB,
	err error,
) *Halt {
	tb.Helper()
	var h *Halt
	require.ErrorAs(tb, err, &h)
	return h
}
