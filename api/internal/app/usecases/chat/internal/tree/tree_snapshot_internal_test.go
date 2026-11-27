package tree

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

func newTestUsecase(t *testing.T) *chatFolderUsecase {
	t.Helper()
	fake := mocks.NewAgentChatPlacements()
	return &chatFolderUsecase{
		chats: fake, agent: fake, workspaces: mocks.NewAgentWorkspaceGitStatus(),
		folders: mocks.NewFolderStore(), nodes: mocks.NewNodePlacements(),
	}
}

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
	if n.Kind != dtree.KindFolder {
		t.Fatalf("want folder-kind Node, got %s", n.Kind)
	}
}

func TestWriteRow_SkipsAnIDTheSnapshotDoesNotHold(t *testing.T) {
	u := &chatFolderUsecase{}
	snapshot := newTreeSnapshotScoped(nil, nil, nil)

	row, err := u.writeRow(context.Background(), snapshot, "ghost")

	require.NoError(t, err)
	assert.Nil(t, row)
}

func TestDrop_IgnoresAnIDTheSnapshotDoesNotHold(t *testing.T) {
	snapshot := newTreeSnapshotScoped(nil, nil, nil)

	snapshot.drop("ghost")

	assert.Empty(t, snapshot.rows)
}
