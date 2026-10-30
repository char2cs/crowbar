package tree_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repotree "github.com/char2cs/crowbar/api/internal/app/repositories/tree"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

type heard struct {
	mu        sync.Mutex
	placement []string
	moved     []string
}

func (h *heard) watch(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.placement = append(h.placement, id)
}

func (h *heard) chat(_ context.Context, chatID string, p tree.Placement, kind string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.moved = append(h.moved, chatID+"@"+p.ParentID+":"+kind)
}

func (h *heard) take() (placement, moved []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	placement, moved = h.placement, h.moved
	h.placement, h.moved = nil, nil
	return placement, moved
}

func TestAnnounce_AMoveCallsThePlacementWatchAndReemitsTheChatFrame(t *testing.T) {
	ctx := context.Background()
	ax := openTree(t, t.TempDir())
	h := &heard{}
	require.NoError(t, repotree.Announce(ax, h.watch, h.chat))
	repo := repotree.New(ax)

	_, err := repo.PutChat(ctx, "p1", "c-q", tree.Placement{ParentID: "f-1"})
	require.NoError(t, err)
	placement, moved := h.take()
	assert.Equal(t, []string{"c-q"}, placement)
	assert.Equal(t, []string{"c-q@f-1:placement_set"}, moved, "heard before the write returns")

	_, err = repo.PutChat(ctx, "p1", "c-q", tree.Placement{ParentID: "f-1", Order: 2})
	require.NoError(t, err)
	placement, moved = h.take()
	assert.Empty(t, placement, "a renumber moves nothing to another parent")
	assert.Equal(t, []string{"c-q@f-1:order_set"}, moved)

	_, err = repo.Put(ctx, "p1", tree.Node{ID: "ws-1", Kind: tree.KindWorkspace, ParentID: "f-1"})
	require.NoError(t, err)
	_, err = repo.Delete(ctx, "p1", "ws-1")
	require.NoError(t, err)
	placement, moved = h.take()
	assert.Equal(t, []string{"ws-1", "ws-1"}, placement, "a position row created, then removed")
	assert.Empty(t, moved, "a position row is not a chat's own placement")
}
