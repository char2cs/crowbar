package ledger_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
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

func TestRead_ABlobRefIsInUseWhileAToolCallCarriesIt(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	call := func(chatID, input, result string) {
		r, err := ledger.NewRow(ledger.KindToolCall, "T1", "", ledger.ToolCall{
			ToolID: "tool-1", Phase: ledger.ToolPost, InputRef: input, ResultRef: result,
		})
		require.NoError(t, err)
		_, _, err = s.Append(ctx, chatID, r)
		require.NoError(t, err)
	}
	call("c1", "sha256:in", "sha256:out")
	call("c2", "sha256:in", "")

	refs, err := s.BlobRefs(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, []string{"sha256:in", "sha256:out"}, refs)

	require.NoError(t, s.Forget(ctx, "c1"))
	inUse, err := s.RefInUse(ctx, "sha256:in")
	require.NoError(t, err)
	assert.True(t, inUse, "another chat still carries it")
	inUse, err = s.RefInUse(ctx, "sha256:out")
	require.NoError(t, err)
	assert.False(t, inUse)
}

func TestRead_OpenChatsListsOnlyChatsWithOpenWork(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, _, err := s.Append(ctx, "c1", prompt(t, "r1", "a"))
	require.NoError(t, err)
	_, _, err = s.Append(ctx, "c2", prompt(t, "r2", "b"), settle(t, "r2"))
	require.NoError(t, err)
	_, _, err = s.Append(ctx, "c3", prompt(t, "r3", "c"))
	require.NoError(t, err)
	_, _, err = s.Append(ctx, "c3", started(t, "r3"))
	require.NoError(t, err)

	open, err := s.OpenChats(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"c1", "c3"}, open, "a pending prompt and a running turn are open; a settled prompt is not")
}
