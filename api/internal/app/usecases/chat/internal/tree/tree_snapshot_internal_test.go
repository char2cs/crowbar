package tree

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// newTestUsecase builds the concrete usecase over an in-memory Chats+Agent
// fake, exposed unwrapped (rather than behind the Usecase interface) so a test
// can reach the chats field directly — the only way to prove a folder is a row
// in that same store rather than a separate table.
func newTestUsecase(t *testing.T) *chatFolderUsecase {
	t.Helper()
	fake := mocks.NewAgentChatPlacements()
	return &chatFolderUsecase{
		chats: fake, agent: fake,
		folders: mocks.NewFolderStore(), nodes: mocks.NewNodePlacements(),
	}
}

// A folder created through the new API is a domain.Folder+domain.Node pair,
// not a row in the chat repository at all (2026-09-08
// sidebar-placement-unification Task 5 for home-scoped, Task 8 for
// repo-scoped too) — Create mints it through the Folders/Nodes ports
// instead, and it carries no workspace — see the model spec §3.1.
func TestTree_Create_MintsFolderPlusNode(t *testing.T) {
	uc := newTestUsecase(t)
	ctx := context.Background()

	folder, _, err := uc.Create(ctx, CreateInput{RepoID: "repo-1", Name: "My Folder"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := uc.folders.FindByKey(ctx, folder.ID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil {
		t.Fatalf("want a Folder row, got none")
	}
	if got.RepoID != "repo-1" {
		t.Fatalf("want repo-1, got %q", got.RepoID)
	}
	if _, err := uc.chats.Get(ctx, folder.ID); err == nil {
		t.Fatalf("a folder must never be a Chat row")
	}
	n, err := uc.nodes.GetNode(ctx, folder.ID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if n.Kind != domain.NodeKindFolder {
		t.Fatalf("want folder-kind Node, got %s", n.Kind)
	}
}

// A dirty id the snapshot does not hold is SKIPPED rather than dereferenced.
// Nothing in the usecase can currently produce one — the plan is built from the
// snapshot's own rows and a dropped row leaves the dirty set with it — but the
// two are separate structures, and the cost of them disagreeing is a nil
// dereference inside a write loop that has already persisted half a move. The
// guard is cheap; discovering it was missing would not be.
//
// It is exercised from inside the package because there is no way to ask for
// this from outside: the seam only exists between the plan and the rows.
func TestWriteRow_SkipsAnIDTheSnapshotDoesNotHold(t *testing.T) {
	u := &chatFolderUsecase{}
	snapshot := newTreeSnapshot(nil)

	row, err := u.writeRow(context.Background(), snapshot, "ghost")

	require.NoError(t, err)
	assert.Nil(t, row)
}

// drop is called for every row a cascade removes, and a cascade can be handed
// the same id twice by two levels of the walk. The second call must be a no-op,
// not a rebuild of the index map over a row that is already gone.
func TestDrop_IgnoresAnIDTheSnapshotDoesNotHold(t *testing.T) {
	snapshot := newTreeSnapshot(nil)

	snapshot.drop("ghost")

	assert.Empty(t, snapshot.rows)
}
