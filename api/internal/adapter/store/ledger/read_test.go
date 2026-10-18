package ledger_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRead_PagesAfterASeq(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, _, err := s.Append(ctx, "c1",
		prompt(t, "r1", "a"), started(t, "r1"), prompt(t, "r2", "b"), settle(t, "r2"), prompt(t, "r3", "c"))
	require.NoError(t, err)

	page, err := s.Read(ctx, "c1", 1, 2)
	require.NoError(t, err)
	assert.Equal(t, []int64{2, 3}, seqs(page))

	rest, err := s.Read(ctx, "c1", 3, 0)
	require.NoError(t, err)
	assert.Equal(t, []int64{4, 5}, seqs(rest))

	none, err := s.Read(ctx, "c1", 5, 10)
	require.NoError(t, err)
	assert.Empty(t, none)

	unknown, err := s.Read(ctx, "nobody", 0, 0)
	require.NoError(t, err)
	assert.Empty(t, unknown)
}
