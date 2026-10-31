package tree_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/char2cs/asynx"
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
	all, err := p.ChatPlacements(ctx, []string{"c-q"})
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

// races is how many times each concurrency test races its two writes.
const races = 200

// raceEach runs a and b concurrently, races times, after reset each time, and
// counts the rounds where stale reports a lost write.
func raceEach(reset, a, b func(), stale func() bool) int {
	lost := 0
	for range races {
		reset()
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); a() }()
		go func() { defer wg.Done(); b() }()
		wg.Wait()
		if stale() {
			lost++
		}
	}
	return lost
}

// An order write changes only the order of the placement the tree holds when
// it lands, so it never restates a parent a concurrent move replaced.
func TestPorts_AnOrderWriteNeverUndoesAConcurrentChatMove(t *testing.T) {
	ctx, repo := context.Background(), newRepo(t)
	p := newPorts(t, repo, newWorld())
	place := func(pl tree.Placement) { require.NoError(t, p.SetChatPlacement(ctx, "c-q", "ws-alpha", pl)) }

	lost := raceEach(
		func() { place(tree.Placement{ParentID: "f-old"}) },
		func() { place(tree.Placement{ParentID: "f-new", Order: 5}) },
		func() { require.NoError(t, p.SetChatOrder(ctx, "c-q", "ws-alpha", 1)) },
		func() bool {
			got, err := p.ChatPlacement(ctx, "c-q")
			require.NoError(t, err)
			return got.ParentID != "f-new"
		})

	assert.Zero(t, lost, "rounds where the move was undone")
}

func TestPorts_AnOrderWriteNeverUndoesAConcurrentNodeMove(t *testing.T) {
	ctx, repo := context.Background(), newRepo(t)
	p := newPorts(t, repo, newWorld())
	_, err := p.Create(ctx, "ws-alpha", tree.KindWorkspace, "", 0)
	require.NoError(t, err)

	lost := raceEach(
		func() { require.NoError(t, p.SetPlacement(ctx, "ws-alpha", "f-old", 0)) },
		func() { require.NoError(t, p.SetPlacement(ctx, "ws-alpha", "f-new", 5)) },
		func() { require.NoError(t, p.SetOrder(ctx, "ws-alpha", 1)) },
		func() bool {
			n, err := p.GetNode(ctx, "ws-alpha")
			require.NoError(t, err)
			return n.ParentID != "f-new"
		})

	assert.Zero(t, lost, "rounds where the move was undone")
}

// Two creates of one row race: exactly one wins, the other is refused.
func TestPorts_ConcurrentCreatesOfOneRowRefuseTheSecond(t *testing.T) {
	ctx, repo := context.Background(), newRepo(t)
	p := newPorts(t, repo, newWorld())
	var refused int
	var mu sync.Mutex
	create := func(parent string) func() {
		return func() {
			if _, err := p.Create(ctx, "ws-alpha", tree.KindWorkspace, parent, 0); err != nil {
				require.ErrorIs(t, err, asynxModels.ErrValidation)
				mu.Lock()
				refused++
				mu.Unlock()
			}
		}
	}

	raceEach(func() { require.NoError(t, p.Forget(ctx, "ws-alpha")) }, create("a"), create("b"), func() bool { return false })

	assert.Equal(t, races, refused, "one create of every round is refused")
}

// Two deletes of one row race: both answer nil, as a delete of a row already
// gone does.
func TestPorts_ConcurrentDeletesOfOneRowAreBothNoError(t *testing.T) {
	ctx, repo := context.Background(), newRepo(t)
	p := newPorts(t, repo, newWorld())
	var mu sync.Mutex
	var errs []error
	record := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, err)
		}
	}
	reset := func() {
		_, err := p.Create(ctx, "ws-alpha", tree.KindWorkspace, "", 0)
		require.NoError(t, err)
		require.NoError(t, p.Save(ctx, tree.Folder{ID: "f-1", Name: "F", HomeID: "home-p1"}))
	}
	forget := func() { record(p.Forget(ctx, "ws-alpha")); record(p.Delete(ctx, "f-1")) }

	raceEach(reset, forget, forget, func() bool { return false })

	assert.Empty(t, errs)
}

// countingGet counts the Tree reads that reach the store.
type countingGet struct {
	asynx.Asynx[tree.Tree]
	reads *atomic.Int64
}

func (c countingGet) Get(ctx context.Context, id string) (tree.Tree, error) {
	c.reads.Add(1)
	return c.Asynx.Get(ctx, id)
}

// The placements of some chats read only the trees holding them, each once.
func TestPorts_ChatPlacementsReadsOnlyTheTreesHoldingTheChats(t *testing.T) {
	ctx := context.Background()
	var reads atomic.Int64
	repo := repotree.New(countingGet{Asynx: openTree(t, t.TempDir()), reads: &reads})
	p := newPorts(t, repo, newWorld())
	require.NoError(t, p.SetChatPlacement(ctx, "c-q", "ws-alpha", tree.Placement{ParentID: "f-1", Order: 2}))
	require.NoError(t, p.SetChatPlacement(ctx, "c-notes", "home-p1", tree.Placement{Order: 1}))
	require.NoError(t, p.SetChatPlacement(ctx, "c-other", "home-p2", tree.Placement{Order: 3}))
	reads.Store(0)

	got, err := p.ChatPlacements(ctx, []string{"c-q", "c-notes", "c-never"})

	require.NoError(t, err)
	assert.Equal(t, map[string]tree.Placement{"c-q": {ParentID: "f-1", Order: 2}, "c-notes": {Order: 1}}, got)
	assert.Equal(t, int64(1), reads.Load(), "both chats are p1's: one read")
}
