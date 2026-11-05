package tree_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/tree"
	"github.com/char2cs/crowbar/api/internal/domain"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

type folderMap map[string]dtree.Folder

func (m folderMap) FindByKey(_ context.Context, id string) (*dtree.Folder, error) {
	f, ok := m[id]
	if !ok {
		return nil, nil
	}
	return &f, nil
}

// A folder nests under the root, another folder, or an owning chat that has no Node.
func TestFoldersBelow_FindsFoldersUnderTheRootFoldersAndOwningChats(t *testing.T) {
	byParent := map[string][]dtree.Node{
		"":      {{ID: "f1", Kind: dtree.KindFolder}, {ID: "c1", Kind: dtree.KindChat}},
		"f1":    {{ID: "f2", Kind: dtree.KindFolder, ParentID: "f1", Order: 2}},
		"owner": {{ID: "f3", Kind: dtree.KindFolder, ParentID: "owner"}, {ID: "gone", Kind: dtree.KindFolder}},
	}
	folders := folderMap{"f1": {ID: "f1", Name: "One"}, "f2": {ID: "f2", Name: "Two", RepoID: "r1"}, "f3": {ID: "f3"}}

	found, err := tree.FoldersBelow(context.Background(), folders, byParent, []domain.Chat{{ID: "owner"}})
	require.NoError(t, err)

	ids := make([]string, len(found))
	for i, f := range found {
		ids[i] = f.ID
	}
	assert.ElementsMatch(t, []string{"f1", "f2", "f3"}, ids, "a node whose folder is gone is skipped")
	assert.Contains(t, found, domain.Chat{
		ID: "f2", Type: domain.ChatTypeFolder, RepoID: "r1", Title: "Two", ParentID: "f1", Order: 2,
	})
}

func TestFoldersBelow_NoReaderFindsNone(t *testing.T) {
	byParent := map[string][]dtree.Node{"": {{ID: "f1", Kind: dtree.KindFolder}}}

	found, err := tree.FoldersBelow(context.Background(), nil, byParent, nil)
	require.NoError(t, err)
	assert.Empty(t, found)
}
