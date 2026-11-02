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
	mu      sync.Mutex
	moved   []string
	folders []string
}

func (h *heard) folder(_ context.Context, projectID string, f tree.Folder, at tree.Node, gone bool, rev int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	state := f.Name + "@" + at.ParentID
	if gone {
		state = "gone"
	}
	h.folders = append(h.folders, projectID+"/"+f.ID+":"+state)
	if rev <= 0 {
		h.folders = append(h.folders, "unversioned")
	}
}

func (h *heard) takeFolders() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	folders := h.folders
	h.folders = nil
	return folders
}

func (h *heard) chat(_ context.Context, chatID string, p tree.Placement, kind string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.moved = append(h.moved, chatID+"@"+p.ParentID+":"+kind)
}

func (h *heard) take() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	moved := h.moved
	h.moved = nil
	return moved
}

func TestAnnounce_AMoveReemitsTheChatFrame(t *testing.T) {
	ctx := context.Background()
	ax := openTree(t, t.TempDir())
	h := &heard{}
	require.NoError(t, repotree.Announce(ax, h.chat, h.folder))
	repo := repotree.New(ax)

	_, err := repo.PutChat(ctx, "p1", "c-q", tree.Placement{ParentID: "f-1"})
	require.NoError(t, err)
	assert.Equal(t, []string{"c-q@f-1:placement_set"}, h.take(), "heard before the write returns")

	_, err = repo.PutChat(ctx, "p1", "c-q", tree.Placement{ParentID: "f-1", Order: 2})
	require.NoError(t, err)
	assert.Equal(t, []string{"c-q@f-1:order_set"}, h.take())

	_, err = repo.Put(ctx, "p1", tree.Node{ID: "ws-1", Kind: tree.KindWorkspace, ParentID: "f-1"})
	require.NoError(t, err)
	_, err = repo.Delete(ctx, "p1", "ws-1")
	require.NoError(t, err)
	assert.Empty(t, h.take(), "a position row is not a chat's own placement")
}

func TestAnnounce_AFolderChangeIsHeardAtItsVersion(t *testing.T) {
	ctx := context.Background()
	ax := openTree(t, t.TempDir())
	h := &heard{}
	require.NoError(t, repotree.Announce(ax, h.chat, h.folder))
	repo := repotree.New(ax)

	_, err := repo.PutFolder(ctx, "p1", tree.Folder{ID: "f-1", Name: "Docs", RepoID: "r1"})
	require.NoError(t, err)
	_, err = repo.Put(ctx, "p1", tree.Node{ID: "f-1", Kind: tree.KindFolder, ParentID: "f-0"})
	require.NoError(t, err)
	assert.Equal(t, []string{"p1/f-1:Docs@", "p1/f-1:Docs@f-0"}, h.takeFolders())

	_, err = repo.PutChat(ctx, "p1", "c-q", tree.Placement{ParentID: "f-1"})
	require.NoError(t, err)
	assert.Empty(t, h.takeFolders(), "a write that leaves the folder as it was")

	_, err = repo.DeleteFolder(ctx, "p1", "f-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"p1/f-1:gone"}, h.takeFolders())
}
