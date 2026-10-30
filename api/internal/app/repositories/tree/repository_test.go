package tree_test

import (
	"context"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

func pinTree() tree.Tree {
	t := tree.Empty("p1")
	t.Nodes["c-notes"] = tree.Node{ID: "c-notes", Kind: tree.KindChat, Order: 0}
	t.Nodes["f-ideas"] = tree.Node{ID: "f-ideas", Kind: tree.KindFolder, Order: 2}
	t.Folders["f-ideas"] = tree.Folder{ID: "f-ideas", Name: "Ideas", HomeID: "home-p1"}
	t.Chats["c-draft"] = tree.Placement{ParentID: "f-ideas", Order: 0}
	return t
}

func TestRepository_SeedThenGetReturnsTheTree(t *testing.T) {
	ctx, repo := context.Background(), newRepo(t)

	_, err := repo.Seed(ctx, pinTree())
	require.NoError(t, err)

	got, err := repo.Get(ctx, "p1")
	require.NoError(t, err)
	assert.Equal(t, pinTree(), got)
}

func TestRepository_SeedMergesIntoAnExistingTree(t *testing.T) {
	ctx, repo := context.Background(), newRepo(t)
	_, err := repo.Put(ctx, "p1", tree.Node{ID: "c-notes", Kind: tree.KindChat, ParentID: "f-ideas", Order: 3})
	require.NoError(t, err)

	got, err := repo.Seed(ctx, pinTree())
	require.NoError(t, err)

	assert.Equal(t, tree.Node{ID: "c-notes", Kind: tree.KindChat, ParentID: "f-ideas", Order: 3}, got.Nodes["c-notes"],
		"a node already present is kept as written")
	assert.Equal(t, "Ideas", got.Folders["f-ideas"].Name, "an absent record is added")
	assert.Equal(t, tree.Placement{ParentID: "f-ideas"}, got.Chats["c-draft"])
}

func TestRepository_PutCreatesThenRepositionsANode(t *testing.T) {
	ctx, repo := context.Background(), newRepo(t)

	_, err := repo.Put(ctx, "p1", tree.Node{ID: "ws-1", Kind: tree.KindWorkspace})
	require.NoError(t, err)
	got, err := repo.Put(ctx, "p1", tree.Node{ID: "ws-1", Kind: tree.KindWorkspace, ParentID: "f-1", Order: 2})
	require.NoError(t, err)

	assert.Equal(t, tree.Node{ID: "ws-1", Kind: tree.KindWorkspace, ParentID: "f-1", Order: 2}, got.Nodes["ws-1"])
	assert.Len(t, got.Nodes, 1)
}

func TestRepository_PutRefusesANodeItCannotPlace(t *testing.T) {
	ctx, repo := context.Background(), newRepo(t)
	for name, n := range map[string]tree.Node{
		"no id":          {Kind: tree.KindChat},
		"unknown kind":   {ID: "n1", Kind: "bogus"},
		"under itself":   {ID: "n1", Kind: tree.KindChat, ParentID: "n1"},
		"negative order": {ID: "n1", Kind: tree.KindChat, Order: -1},
	} {
		_, err := repo.Put(ctx, "p1", n)
		require.ErrorIs(t, err, asynxModels.ErrValidation, name)
	}
	got, err := repo.Get(ctx, "p1")
	require.NoError(t, err)
	assert.Empty(t, got.Nodes)
}

func TestRepository_DeleteOfAnUnknownNodeIsNotFound(t *testing.T) {
	ctx, repo := context.Background(), newRepo(t)
	_, err := repo.Seed(ctx, pinTree())
	require.NoError(t, err)

	_, err = repo.Delete(ctx, "p1", "nope")

	require.ErrorIs(t, err, tree.ErrUnknownNode)
	assert.Equal(t, problem.NotFound, problem.CodeOf(err))
}

func TestRepository_DeleteRemovesOnlyThePositionRow(t *testing.T) {
	ctx, repo := context.Background(), newRepo(t)
	_, err := repo.Seed(ctx, pinTree())
	require.NoError(t, err)

	got, err := repo.Delete(ctx, "p1", "f-ideas")
	require.NoError(t, err)

	assert.NotContains(t, got.Nodes, "f-ideas")
	assert.Contains(t, got.Folders, "f-ideas", "the folder's identity is its own record")
}

func TestRepository_GetOfAnAbsentProjectIsAnEmptyTree(t *testing.T) {
	got, err := newRepo(t).Get(context.Background(), "p-none")

	require.NoError(t, err)
	assert.Equal(t, tree.Empty("p-none"), got)
}

func TestRepository_ForgetRemovesTheTree(t *testing.T) {
	ctx, repo := context.Background(), newRepo(t)
	_, err := repo.Seed(ctx, pinTree())
	require.NoError(t, err)

	require.NoError(t, repo.Forget(ctx, "p1"))
	require.NoError(t, repo.Forget(ctx, "p1"), "forgetting a tree that is gone is no error")

	got, err := repo.Get(ctx, "p1")
	require.NoError(t, err)
	assert.Equal(t, tree.Empty("p1"), got)
}

func TestRepository_ATreeSurvivesARestart(t *testing.T) {
	ctx, dir := context.Background(), t.TempDir()
	_, err := repotreeOver(t, dir).Seed(ctx, pinTree())
	require.NoError(t, err)

	got, err := repotreeOver(t, dir).Get(ctx, "p1")

	require.NoError(t, err)
	assert.Equal(t, pinTree(), got)
}
