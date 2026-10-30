package migratetree

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	repotree "github.com/char2cs/crowbar/api/internal/app/repositories/tree"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
	"github.com/char2cs/crowbar/api/internal/testutil"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

const stepNumber = 7

// ports opens the Trees under l's home, placed by l's facts as the live
// Locator places them.
func (l *legacy) ports(t *testing.T) *repotree.Ports {
	t.Helper()
	ctx := context.Background()
	es, err := eventsqlite.NewEventStore(filepath.Join(l.h.Events(), "tree.db"))
	require.NoError(t, err)
	ss, err := eventsqlite.NewSnapshotStore(filepath.Join(l.h.Events(), "tree_snapshots.db"))
	require.NoError(t, err)
	ax, err := asynx.New[tree.Tree]().WithEventStore(es).WithSnapshotStore(ss).Build()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, ax.Shutdown(ctx))
		closeLog(ctx, es)
	})
	find := func(m map[string]string) func(context.Context, string) (string, error) {
		return func(_ context.Context, id string) (string, error) {
			if v, ok := m[id]; ok {
				return v, nil
			}
			return "", problem.ErrNotFound
		}
	}
	chatWS, wsProject, repoProject := map[string]string{}, map[string]string{}, map[string]string{}
	for _, w := range l.workspaces {
		wsProject[w.ID] = w.ProjectID
	}
	for _, r := range l.repos {
		repoProject[r.ID] = r.ProjectID
	}
	h, err := ReadLegacy(ctx, l.sources())
	require.NoError(t, err)
	for _, c := range h.Chats {
		chatWS[c.ID] = c.WorkspaceID
	}
	locate := repotree.NewLocator(repotree.LocatorFacts{
		ChatWorkspace: find(chatWS), WorkspaceProject: find(wsProject), RepoProject: find(repoProject),
	})
	ids := make([]string, len(l.projects))
	for i, p := range l.projects {
		ids[i] = p.ID
	}
	p, err := repotree.NewPorts(ctx, repotree.New(ax), locate, ids)
	require.NoError(t, err)
	return p
}

// boot runs the step alone through the engine, as a daemon's post-open half.
func (l *legacy) boot(t *testing.T, ports *repotree.Ports) error {
	t.Helper()
	plan := func(context.Context, migrate.Env) (migrate.Plan, error) { return migrate.Plan{}, nil }
	cat, err := migrate.NewCatalog(migrate.Step{Number: stepNumber, Name: "placement-tree", Phase: migrate.PostOpen, Plan: plan})
	require.NoError(t, err)
	sess, err := migrate.Begin(context.Background(), l.h, cat, migrate.Options{Version: "v-test"})
	require.NoError(t, err)
	defer func() { require.NoError(t, sess.Close()) }()
	require.NoError(t, sess.RunPreOpen(context.Background(), migrate.Impls{}))
	return sess.RunPostOpen(context.Background(), migrate.Impls{stepNumber: PlacementTreeImpl(ports, l.sources())})
}

// pinned is a home holding every legacy placement shape: two projects (p1 the
// older), a repo with a branch and a fork, and folders of every vintage.
func pinned(t *testing.T) *legacy {
	t.Helper()
	l := newLegacy(t)
	l.project("p1", "h1", time.Unix(100, 0))
	l.project("p2", "h2", time.Unix(200, 0))
	l.repo("r1", "p1")
	l.workspace("ws-a", "p1", "r1")
	l.workspace("ws-fork", "p1", "r1")
	l.folder(t, tree.Folder{ID: "f-repo", Name: "Repo F", RepoID: "r1"})
	l.folder(t, tree.Folder{ID: "f-home", Name: "Home F", HomeID: "h1"})
	l.folder(t, tree.Folder{ID: "f-legacy", Name: "Old"})
	l.folder(t, tree.Folder{ID: "f-orphan", Name: "Lonely"})
	l.chat(t, "c-home", "h1", "", 0)
	l.chat(t, "c-repo", "ws-a", "f-repo", 1)
	l.chat(t, "c-owner", "ws-fork", "", 0)
	l.chat(t, "c-tie1", "ws-a", "f-repo", 5)
	l.chat(t, "c-tie2", "ws-a", "f-repo", 5)
	l.chat(t, "c-in-legacy", "h2", "f-legacy", 0)
	l.chat(t, "c-dangle", "h1", "gone", 2)
	l.nodes(t,
		legacyNode{ID: "c-home", Kind: "chat", ParentID: "f-home", Order: 2},
		legacyNode{ID: "f-home", Kind: "folder", Order: 1},
		legacyNode{ID: "r1", Kind: "repo", ParentID: "f-home", Order: 4},
		legacyNode{ID: "ws-a", Kind: "workspace"},
		legacyNode{ID: "c-owner", Kind: "chat", ParentID: "r1", Order: 3},
	)
	return l
}

func pinnedTrees() (p1, p2 tree.Tree) {
	p1, p2 = tree.Empty("p1"), tree.Empty("p2")
	p1.Nodes = map[string]tree.Node{
		"c-home":  {ID: "c-home", Kind: tree.KindChat, ParentID: "f-home", Order: 2},
		"f-home":  {ID: "f-home", Kind: tree.KindFolder, Order: 1},
		"r1":      {ID: "r1", Kind: tree.KindRepo, ParentID: "f-home", Order: 4},
		"ws-a":    {ID: "ws-a", Kind: tree.KindWorkspace},
		"c-owner": {ID: "c-owner", Kind: tree.KindChat, ParentID: "r1", Order: 3},
	}
	p1.Folders = map[string]tree.Folder{
		"f-repo":   {ID: "f-repo", Name: "Repo F", RepoID: "r1"},
		"f-home":   {ID: "f-home", Name: "Home F", HomeID: "h1"},
		"f-orphan": {ID: "f-orphan", Name: "Lonely", HomeID: "h1"},
	}
	p1.Chats = map[string]tree.Placement{
		"c-home": {}, "c-owner": {}, "c-dangle": {Order: 2},
		"c-repo": {ParentID: "f-repo", Order: 1},
		"c-tie1": {ParentID: "f-repo", Order: 5}, "c-tie2": {ParentID: "f-repo", Order: 5},
	}
	p2.Folders = map[string]tree.Folder{"f-legacy": {ID: "f-legacy", Name: "Old", HomeID: "h2"}}
	p2.Chats = map[string]tree.Placement{"c-in-legacy": {ParentID: "f-legacy"}}
	return p1, p2
}

func treeOf(t *testing.T, p *repotree.Ports, id string) tree.Tree {
	t.Helper()
	got, err := p.Tree(context.Background(), id)
	require.NoError(t, err)
	return got
}

// Review Focus 1: every legacy shape survives verbatim.
func TestPlacementTreeStep_CopiesEveryLegacyPlacement(t *testing.T) {
	l := pinned(t)
	ports := l.ports(t)

	require.NoError(t, l.boot(t, ports))

	wantP1, wantP2 := pinnedTrees()
	assert.Equal(t, wantP1, treeOf(t, ports, "p1"),
		"a home folder before HomeID with no neighbour goes to the oldest project; a dangling parent hangs at the root")
	assert.Equal(t, wantP2, treeOf(t, ports, "p2"), "a home folder before HomeID goes to the home its chat names")
	assert.Equal(t, tree.Empty(tree.Unplaced), treeOf(t, ports, tree.Unplaced))
	applied := migratetest.Applied(t, l.h)[stepNumber]
	require.Len(t, applied, 1)
	assert.Equal(t, migrate.Counts{"nodes": 5, "folders": 4, "chats": 7, "rehung_to_root": 1}, applied[0])
}

func TestPlacementTreeStep_LeavesTheNodeFilesAndTheFoldersTableUnread(t *testing.T) {
	l := pinned(t)
	ports := l.ports(t)
	before := testutil.TreeSnapshot(t, l.h)

	require.NoError(t, l.boot(t, ports))

	after := testutil.TreeSnapshot(t, l.h)
	for _, kept := range []string{"state/events/node.db", "state/view.db"} {
		assert.Contains(t, after, entryOf(t, before, kept), "the step deletes and rewrites nothing it copied from")
	}
	var folders int64
	require.NoError(t, l.view.Raw("SELECT count(*) FROM folders").Scan(&folders).Error)
	assert.Equal(t, int64(4), folders)
}

func entryOf(t *testing.T, snapshot []string, rel string) string {
	t.Helper()
	for _, e := range snapshot {
		if len(e) > len(rel) && e[:len(rel)+1] == rel+" " {
			return e
		}
	}
	t.Fatalf("%s is not in the home", rel)
	return ""
}

func TestPlacementTreeStep_SeedsOverATreeAnEarlierStepWrote(t *testing.T) {
	l := pinned(t)
	ports := l.ports(t)
	_, err := ports.Create(context.Background(), "c-home", tree.KindChat, "f-home", 9)
	require.NoError(t, err)

	require.NoError(t, l.boot(t, ports))

	got := treeOf(t, ports, "p1")
	assert.Equal(t, 9, got.Nodes["c-home"].Order, "a row written since the legacy read is kept as written")
	assert.Contains(t, got.Nodes, "r1", "the rest is still copied")
}

func TestPlacementTreeStep_IsIdempotentAfterACrashMidway(t *testing.T) {
	l := pinned(t)
	ports := l.ports(t)
	wantP1, wantP2 := pinnedTrees()
	_, err := ports.Seed(context.Background(), wantP1)
	require.NoError(t, err)
	testutil.WriteFile(t, l.h, "migrate/ledger.jsonl",
		`{"v":1,"kind":"run.start","run":"r1","pending":[7]}`+"\n"+
			`{"v":1,"kind":"checkpoint","run":"r1","step":7,"name":"placement-tree","key":"project:p1","counts":{"nodes":5}}`+"\n")

	require.NoError(t, l.boot(t, ports))

	assert.Equal(t, wantP2, treeOf(t, ports, "p2"))
	assert.Equal(t, []migrate.Counts{{"nodes": 5, "folders": 1, "chats": 1}}, migratetest.Applied(t, l.h)[stepNumber],
		"project 1, marked by the stopped attempt, is not counted again")
	before := testutil.TreeSnapshot(t, l.h)
	require.NoError(t, l.boot(t, ports))
	assert.Equal(t, before, testutil.TreeSnapshot(t, l.h), "a second boot copies nothing")
}

func TestPlacementTreeStep_VerifyFailsOnAChatWithoutANode(t *testing.T) {
	l := pinned(t)
	ports := l.ports(t)
	p1, p2 := pinnedTrees()
	delete(p1.Chats, "c-repo")
	for _, tr := range []tree.Tree{p1, p2} {
		_, err := ports.Seed(context.Background(), tr)
		require.NoError(t, err)
	}

	err := PlacementTreeImpl(ports, l.sources()).Verify(context.Background(), migrate.Env{Home: l.h})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat c-repo")
}
