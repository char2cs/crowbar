package tree_test

import (
	"context"
	"fmt"
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

// Reads racing writes: once a write returns, every read holds what it wrote,
// and a read never hands out the tree a later read could change under it.
func TestRepository_EveryReadAfterAWriteHoldsIt(t *testing.T) {
	ctx, repo := context.Background(), newRepo(t)
	const writes = 100
	var wg sync.WaitGroup
	wg.Add(2)
	stop := make(chan struct{})
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			got, err := repo.Get(ctx, "p1")
			assert.NoError(t, err)
			got.Nodes["scribble"] = tree.Node{ID: "scribble"}
		}
	}()
	go func() {
		defer wg.Done()
		defer close(stop)
		for i := range writes {
			id := fmt.Sprintf("n-%d", i)
			_, err := repo.Put(ctx, "p1", tree.Node{ID: id, Kind: tree.KindChat})
			assert.NoError(t, err)
			got, err := repo.Get(ctx, "p1")
			assert.NoError(t, err)
			assert.Contains(t, got.Nodes, id, "a read after the write holds it")
		}
	}()
	wg.Wait()

	got, err := repo.Get(ctx, "p1")
	require.NoError(t, err)
	assert.Len(t, got.Nodes, writes)
	assert.NotContains(t, got.Nodes, "scribble", "a caller's edit to its copy is its own")
}

// B commits after A but its caller gives up waiting for handlers (ctx cancel
// after commit) and evicts before A's hold runs: A then holds an older tree.
func TestRepository_AWriteThatFailsAfterItsCommitDropsAnOlderWritesHold(t *testing.T) {
	ax := openTree(t, t.TempDir())
	release := make(chan struct{})
	entered := make(chan string, 4)
	_, err := ax.Subscribe(asynx.Topic("tree.*"), func(_ context.Context, evt asynxModels.Event[tree.Tree]) {
		entered <- fmt.Sprint(evt.Version)
		<-release
	})
	require.NoError(t, err)
	repo := repotree.New(ax)
	ctx := context.Background()

	aDone := make(chan error, 1)
	go func() {
		_, err := repo.Put(ctx, "p1", tree.Node{ID: "a", Kind: tree.KindChat})
		aDone <- err
	}()
	<-entered // A committed, its handler is blocked

	bctx, cancel := context.WithCancel(ctx)
	bDone := make(chan error, 1)
	go func() {
		_, err := repo.Put(bctx, "p1", tree.Node{ID: "b", Kind: tree.KindChat})
		bDone <- err
	}()
	// Wait until B committed: the store holds b.
	require.Eventually(t, func() bool {
		got, err := ax.Get(ctx, "p1")
		return err == nil && got.Nodes["b"].ID == "b"
	}, 5e9, 1e6)
	cancel()
	require.Error(t, <-bDone, "B's caller sees a failure though B committed")
	close(release)
	require.NoError(t, <-aDone)

	stored, err := ax.Get(ctx, "p1")
	require.NoError(t, err)
	got, err := repo.Get(ctx, "p1")
	require.NoError(t, err)
	assert.Len(t, got.Nodes, len(stored.Nodes), "held tree must match the store")
	assert.Contains(t, got.Nodes, "b")
}

// A write that committed before Forget, whose hold lands after Forget's evict,
// resurrects a forgotten project's tree.
func TestRepository_AWriteEndingAfterForgetIsNotHeld(t *testing.T) {
	ax := openTree(t, t.TempDir())
	release := make(chan struct{})
	entered := make(chan struct{}, 4)
	_, err := ax.Subscribe(asynx.Topic("tree.*"), func(_ context.Context, _ asynxModels.Event[tree.Tree]) {
		entered <- struct{}{}
		<-release
	})
	require.NoError(t, err)
	repo := repotree.New(ax)
	ctx := context.Background()
	aDone := make(chan error, 1)
	go func() {
		_, err := repo.Put(ctx, "p1", tree.Node{ID: "a", Kind: tree.KindChat})
		aDone <- err
	}()
	<-entered
	fDone := make(chan error, 1)
	go func() { fDone <- repo.Forget(ctx, "p1") }()
	close(release)
	require.NoError(t, <-fDone)
	require.NoError(t, <-aDone)
	got, err := repo.Get(ctx, "p1")
	require.NoError(t, err)
	assert.Empty(t, got.Nodes, "a forgotten project's tree is empty")
}

// Racing writers and readers on several projects: after all writes, every held
// tree equals the store.
func TestRepository_RacingWritersAndReadersLeaveEveryHeldTreeAsStored(t *testing.T) {
	ax := openTree(t, t.TempDir())
	repo := repotree.New(ax)
	ctx := context.Background()
	const rounds = 200
	var wg sync.WaitGroup
	for w := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range rounds {
				p := fmt.Sprintf("p%d", i%3)
				id := fmt.Sprintf("w%d-%d", w, i)
				_, err := repo.Put(ctx, p, tree.Node{ID: id, Kind: tree.KindChat})
				assert.NoError(t, err)
				if i%7 == 0 {
					_, err = repo.Delete(ctx, p, id)
					assert.NoError(t, err)
				}
				if i%11 == 0 {
					_, _ = repo.Take(ctx, p, fmt.Sprintf("w%d-%d", (w+1)%4, i-1))
				}
			}
		}()
	}
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range rounds * 2 {
				got, err := repo.Get(ctx, fmt.Sprintf("p%d", i%3))
				assert.NoError(t, err)
				got.Nodes["scribble"] = tree.Node{ID: "scribble"}
				delete(got.Nodes, "w0-1")
			}
		}()
	}
	wg.Wait()
	for i := range 3 {
		p := fmt.Sprintf("p%d", i)
		stored, err := ax.Get(ctx, p)
		require.NoError(t, err)
		got, err := repo.Get(ctx, p)
		require.NoError(t, err)
		assert.Equal(t, tree.Empty(p).Merge(stored), got, p)
	}
}

// One write whose caller gives up after the commit: the next read holds it.
func TestRepository_AWriteWhoseCallerGaveUpAfterItsCommitIsRead(t *testing.T) {
	ax := openTree(t, t.TempDir())
	release := make(chan struct{})
	entered := make(chan struct{}, 4)
	_, err := ax.Subscribe(asynx.Topic("tree.*"), func(_ context.Context, _ asynxModels.Event[tree.Tree]) {
		entered <- struct{}{}
		<-release
	})
	require.NoError(t, err)
	repo := repotree.New(ax)
	ctx := context.Background()
	go func() { <-entered; close(release) }()
	_, err = repo.Put(ctx, "p1", tree.Node{ID: "a", Kind: tree.KindChat})
	require.NoError(t, err)
	_, err = repo.Get(ctx, "p1")
	require.NoError(t, err)
	release2 := make(chan struct{})
	release = release2
	bctx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { _, err := repo.Put(bctx, "p1", tree.Node{ID: "b", Kind: tree.KindChat}); done <- err }()
	<-entered
	cancel()
	require.Error(t, <-done)
	close(release2)
	got, err := repo.Get(ctx, "p1")
	require.NoError(t, err)
	assert.Contains(t, got.Nodes, "b")
}

type slowGet struct {
	asynx.Asynx[tree.Tree]
	read, resume chan struct{}
}

func (s slowGet) Get(ctx context.Context, id string) (tree.Tree, error) {
	t, err := s.Asynx.Get(ctx, id)
	s.read <- struct{}{}
	<-s.resume
	return t, err
}

// A miss whose read finished before a write landed must not hold what it read.
func TestRepository_AReadThatMissedAWriteIsNotHeld(t *testing.T) {
	ax := openTree(t, t.TempDir())
	ctx := context.Background()
	s := slowGet{Asynx: ax, read: make(chan struct{}), resume: make(chan struct{})}
	repo := repotree.New(s)
	done := make(chan struct{})
	go func() { _, _ = repo.Get(ctx, "p1"); close(done) }()
	<-s.read
	_, err := repo.Put(ctx, "p1", tree.Node{ID: "a", Kind: tree.KindChat})
	require.NoError(t, err)
	repo.Evict("p1") // the slot is empty again, as after a write that may have committed unseen
	close(s.resume)
	<-done
	go func() { <-s.read }()
	got, err := repo.Get(ctx, "p1")
	require.NoError(t, err)
	assert.Contains(t, got.Nodes, "a")
}

// countingGet counts the reads that reach the store.
type countingGet struct {
	asynx.Asynx[tree.Tree]
	reads *atomic.Int64
}

func (c countingGet) Get(ctx context.Context, id string) (tree.Tree, error) {
	c.reads.Add(1)
	return c.Asynx.Get(ctx, id)
}

// A write its command refuses committed nothing, so what is held stays held.
func TestRepository_ARefusedWriteKeepsTheHeldTree(t *testing.T) {
	ctx := context.Background()
	var reads atomic.Int64
	repo := repotree.New(countingGet{Asynx: openTree(t, t.TempDir()), reads: &reads})
	_, err := repo.Put(ctx, "p1", tree.Node{ID: "a", Kind: tree.KindChat})
	require.NoError(t, err)

	_, err = repo.Delete(ctx, "p1", "nope")
	require.ErrorIs(t, err, tree.ErrUnknownNode)

	got, err := repo.Get(ctx, "p1")
	require.NoError(t, err)
	assert.Contains(t, got.Nodes, "a")
	assert.Zero(t, reads.Load(), "the read is answered from what the Put returned")
}

// A read while writes are under way may see a write whose result has not come
// back; it is not held, so a later read never goes back to an older tree.
func TestRepository_AReadDuringWritesNeverMakesALaterReadOlder(t *testing.T) {
	ax := openTree(t, t.TempDir())
	entered, release := make(chan struct{}, 4), make(chan struct{}, 4)
	_, err := ax.Subscribe(asynx.Topic("tree.*"), func(context.Context, asynxModels.Event[tree.Tree]) {
		entered <- struct{}{}
		<-release
	})
	require.NoError(t, err)
	repo := repotree.New(ax)
	ctx := context.Background()
	put := func(id string) chan error {
		done := make(chan error, 1)
		go func() { _, err := repo.Put(ctx, "p1", tree.Node{ID: id, Kind: tree.KindChat}); done <- err }()
		return done
	}
	aDone := put("a")
	<-entered
	bDone := put("b")
	require.Eventually(t, func() bool {
		got, err := ax.Get(ctx, "p1")
		return err == nil && got.Nodes["b"].ID == "b"
	}, 5e9, 1e6)

	first, err := repo.Get(ctx, "p1")
	require.NoError(t, err)
	require.Contains(t, first.Nodes, "b")
	release <- struct{}{}
	require.NoError(t, <-aDone)
	later, err := repo.Get(ctx, "p1")
	require.NoError(t, err)
	release <- struct{}{}
	require.NoError(t, <-bDone)

	assert.Contains(t, later.Nodes, "b", "a later read holds what an earlier one saw")
}

// Two writes come back out of order: the older, ending last, does not fill the
// slot over the newer one's commit.
func TestRepository_AnOlderWriteEndingLastIsNotHeld(t *testing.T) {
	ctx := context.Background()
	repo := repotree.New(openTree(t, t.TempDir()))
	older := repo.Begin("p1")
	_, err := repo.Put(ctx, "p1", tree.Node{ID: "b", Kind: tree.KindChat})
	require.NoError(t, err)

	repo.End("p1", older, asynxModels.Event[tree.Tree]{Version: 0, Aggregate: tree.Empty("p1")}, nil)

	got, err := repo.Get(ctx, "p1")
	require.NoError(t, err)
	assert.Contains(t, got.Nodes, "b")
}

// A write ending after a newer one was held leaves the newer one held.
func TestRepository_AnOlderWriteNeverReplacesANewerHeldTree(t *testing.T) {
	ctx := context.Background()
	repo := repotree.New(openTree(t, t.TempDir()))
	_, err := repo.Put(ctx, "p1", tree.Node{ID: "a", Kind: tree.KindChat})
	require.NoError(t, err)
	older := repo.Begin("p1")
	_, err = repo.Put(ctx, "p1", tree.Node{ID: "b", Kind: tree.KindChat})
	require.NoError(t, err)
	repo.End("p1", older, asynxModels.Event[tree.Tree]{Version: 1, Aggregate: tree.Empty("p1")}, nil)

	got, err := repo.Get(ctx, "p1")
	require.NoError(t, err)
	assert.Contains(t, got.Nodes, "b")
}
