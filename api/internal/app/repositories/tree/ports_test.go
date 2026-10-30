package tree_test

import (
	"context"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repotree "github.com/char2cs/crowbar/api/internal/app/repositories/tree"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// world is the facts a Locator reads: which workspace a chat is in, which
// project a workspace and a repo belong to.
type world struct {
	chatWS, wsProject, repoProject map[string]string
}

func newWorld() *world {
	return &world{
		chatWS:      map[string]string{"c-notes": "home-p1", "c-q": "ws-alpha", "c-other": "home-p2"},
		wsProject:   map[string]string{"home-p1": "p1", "ws-alpha": "p1", "home-p2": "p2"},
		repoProject: map[string]string{"r-alpha": "p1"},
	}
}

func lookup(m map[string]string) func(context.Context, string) (string, error) {
	return func(_ context.Context, id string) (string, error) {
		if v, ok := m[id]; ok {
			return v, nil
		}
		return "", problem.ErrNotFound
	}
}

func (w *world) locator() repotree.Locator {
	return repotree.NewLocator(repotree.LocatorFacts{
		ChatWorkspace:    lookup(w.chatWS),
		WorkspaceProject: lookup(w.wsProject),
		RepoProject:      lookup(w.repoProject),
	})
}

func newPorts(t *testing.T, repo *repotree.Repository, w *world) *repotree.Ports {
	t.Helper()
	p, err := repotree.NewPorts(context.Background(), repo, w.locator(), []string{"p1", "p2"})
	require.NoError(t, err)
	return p
}

func TestPorts_CreateGetListByParentRoundTrip(t *testing.T) {
	ctx, repo := context.Background(), newRepo(t)
	p := newPorts(t, repo, newWorld())

	_, err := p.Create(ctx, "r-alpha", tree.KindRepo, "", 1)
	require.NoError(t, err)
	_, err = p.Create(ctx, "c-notes", tree.KindChat, "", 0)
	require.NoError(t, err)
	_, err = p.Create(ctx, "c-other", tree.KindChat, "", 0)
	require.NoError(t, err)
	require.NoError(t, p.SetPlacement(ctx, "c-notes", "r-alpha", 4))
	require.NoError(t, p.SetOrder(ctx, "c-notes", 2))

	got, err := p.GetNode(ctx, "c-notes")
	require.NoError(t, err)
	assert.Equal(t, tree.Node{ID: "c-notes", Kind: tree.KindChat, ParentID: "r-alpha", Order: 2}, got)
	roots, err := p.ListByParent(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"c-other", "r-alpha"}, ids(roots), `"" is every scope's root, across projects`)

	p1, err := repo.Get(ctx, "p1")
	require.NoError(t, err)
	assert.Contains(t, p1.Nodes, "c-notes")
	assert.NotContains(t, p1.Nodes, "c-other", "each row is stored in its own project's tree")
}

func TestPorts_RefusesWhatTheNodeRowsRefused(t *testing.T) {
	ctx := context.Background()
	p := newPorts(t, newRepo(t), newWorld())
	_, err := p.Create(ctx, "c-notes", tree.KindChat, "", 0)
	require.NoError(t, err)

	_, err = p.Create(ctx, "c-notes", tree.KindChat, "", 1)
	require.ErrorIs(t, err, asynxModels.ErrValidation, "a second create of one id")
	require.ErrorIs(t, p.SetOrder(ctx, "nope", 0), asynxModels.ErrValidation)
	require.ErrorIs(t, p.SetPlacement(ctx, "c-notes", "c-notes", 0), asynxModels.ErrValidation)
	_, err = p.GetNode(ctx, "nope")
	assert.Equal(t, problem.NotFound, problem.CodeOf(err))
	require.NoError(t, p.Forget(ctx, "nope"), "forgetting a row that is not there")
}

func TestPorts_FolderSaveKeepsItsNameApartFromItsPosition(t *testing.T) {
	ctx := context.Background()
	p := newPorts(t, newRepo(t), newWorld())

	require.NoError(t, p.Save(ctx, tree.Folder{ID: "f-ideas", Name: "Ideas", HomeID: "home-p1"}))
	_, err := p.Create(ctx, "f-ideas", tree.KindFolder, "", 2)
	require.NoError(t, err)
	require.NoError(t, p.Save(ctx, tree.Folder{ID: "f-ideas", Name: "Big ideas", HomeID: "home-p1"}))

	f, err := p.FindByKey(ctx, "f-ideas")
	require.NoError(t, err)
	assert.Equal(t, &tree.Folder{ID: "f-ideas", Name: "Big ideas", HomeID: "home-p1"}, f)
	n, err := p.GetNode(ctx, "f-ideas")
	require.NoError(t, err)
	assert.Equal(t, 2, n.Order, "a rename keeps the folder where it is")

	require.NoError(t, p.Delete(ctx, "f-ideas"))
	f, err = p.FindByKey(ctx, "f-ideas")
	require.NoError(t, err)
	assert.Nil(t, f)
	_, err = p.GetNode(ctx, "f-ideas")
	require.NoError(t, err, "the position row is removed on its own")
}

func TestPorts_ChatPlacementWritesTheChatsRecord(t *testing.T) {
	ctx, repo := context.Background(), newRepo(t)
	p := newPorts(t, repo, newWorld())

	require.NoError(t, p.SetChatPlacement(ctx, "c-q", "ws-alpha", tree.Placement{ParentID: "f-1", Order: 3}))
	require.NoError(t, p.SetChatOrder(ctx, "c-q", "ws-alpha", 1))

	got, err := p.ChatPlacement(ctx, "c-q")
	require.NoError(t, err)
	assert.Equal(t, tree.Placement{ParentID: "f-1", Order: 1}, got, "an order write keeps the parent")
	_, err = p.GetNode(ctx, "c-q")
	require.Error(t, err, "a chat's own placement is not a position row")
	all, err := p.ChatPlacements(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]tree.Placement{"c-q": {ParentID: "f-1", Order: 1}}, all)

	require.NoError(t, p.DropChat(ctx, "c-q"))
	got, err = p.ChatPlacement(ctx, "c-q")
	require.NoError(t, err)
	assert.Equal(t, tree.Placement{}, got)
}

func TestPorts_LocateFindsTheProjectOfAFreshChat(t *testing.T) {
	ctx, repo := context.Background(), newRepo(t)
	p := newPorts(t, repo, newWorld())

	_, err := p.Create(ctx, "c-q", tree.KindChat, "", 0)
	require.NoError(t, err)

	p1, err := repo.Get(ctx, "p1")
	require.NoError(t, err)
	assert.Contains(t, p1.Nodes, "c-q", "no row yet: found through its workspace")
}

func TestPorts_AChatPlacedBeforeItHasAWorkspaceMovesWhenItGetsOne(t *testing.T) {
	ctx, repo, w := context.Background(), newRepo(t), newWorld()
	p := newPorts(t, repo, w)

	require.NoError(t, p.SetChatPlacement(ctx, "c-new", "", tree.Placement{Order: 4}))
	unplaced, err := repo.Get(ctx, tree.Unplaced)
	require.NoError(t, err)
	require.Contains(t, unplaced.Chats, "c-new")

	w.chatWS["c-new"] = "ws-alpha"
	require.NoError(t, p.AdoptChat(ctx, "c-new", "ws-alpha"))

	p1, err := repo.Get(ctx, "p1")
	require.NoError(t, err)
	assert.Equal(t, tree.Placement{Order: 4}, p1.Chats["c-new"])
	unplaced, err = repo.Get(ctx, tree.Unplaced)
	require.NoError(t, err)
	assert.NotContains(t, unplaced.Chats, "c-new")
}

func TestPorts_ARowUnderAKnownParentIsPlacedInItsParentsProject(t *testing.T) {
	ctx, repo := context.Background(), newRepo(t)
	p := newPorts(t, repo, newWorld())

	require.NoError(t, p.SetChatPlacement(ctx, "c-thread", "", tree.Placement{ParentID: "c-notes"}))

	p1, err := repo.Get(ctx, "p1")
	require.NoError(t, err)
	assert.Contains(t, p1.Chats, "c-thread")
}

func TestPorts_AReopenFindsEveryRowWhereItWasLeft(t *testing.T) {
	ctx, dir := context.Background(), t.TempDir()
	p := newPorts(t, repotreeOver(t, dir), newWorld())
	_, err := p.Create(ctx, "c-notes", tree.KindChat, "", 0)
	require.NoError(t, err)
	require.NoError(t, p.SetChatPlacement(ctx, "c-new", "", tree.Placement{Order: 2}))

	again := newPorts(t, repotreeOver(t, dir), newWorld())

	n, err := again.GetNode(ctx, "c-notes")
	require.NoError(t, err)
	assert.Equal(t, "c-notes", n.ID)
	pl, err := again.ChatPlacement(ctx, "c-new")
	require.NoError(t, err)
	assert.Equal(t, 2, pl.Order)
}

func TestPorts_SeedKeepsARowWrittenBeforeIt(t *testing.T) {
	ctx := context.Background()
	p := newPorts(t, newRepo(t), newWorld())
	_, err := p.Create(ctx, "c-notes", tree.KindChat, "f-ideas", 5)
	require.NoError(t, err)

	legacy := tree.Empty("p1")
	legacy.Nodes["c-notes"] = tree.Node{ID: "c-notes", Kind: tree.KindChat}
	legacy.Folders["f-ideas"] = tree.Folder{ID: "f-ideas", Name: "Ideas", HomeID: "home-p1"}
	_, err = p.Seed(ctx, legacy)
	require.NoError(t, err)

	n, err := p.GetNode(ctx, "c-notes")
	require.NoError(t, err)
	assert.Equal(t, 5, n.Order)
	f, err := p.FindByKey(ctx, "f-ideas")
	require.NoError(t, err)
	assert.Equal(t, "Ideas", f.Name, "the seeded row is found by id")
}

func ids(nodes []tree.Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.ID
	}
	return out
}
