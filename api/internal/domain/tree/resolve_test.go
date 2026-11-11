package tree_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

func TestResolveWorkspace_WalksToTheNearestWorkspaceThenRepoThenHome(t *testing.T) {
	tr, f := pinTree()
	assert.Equal(t, "ws-feat", tree.ResolveWorkspace(tr, "ws-feat", f))
	assert.Equal(t, "ws-feat", tree.ResolveWorkspace(tr, "c-follow", f), "a chat resolves to its own workspace")
	assert.Equal(t, "home-alpha", tree.ResolveWorkspace(tr, "f-alpha", f), "a repo folder to the repo's home checkout")
	assert.Equal(t, "home-alpha", tree.ResolveWorkspace(tr, "r-alpha", f))
	assert.Equal(t, "home-p1", tree.ResolveWorkspace(tr, "f-ideas", f))
	assert.Equal(t, "home-p1", tree.ResolveWorkspace(tr, "", f))
	moved, err := tr.Move("c-follow", "f-alpha", 0, f)
	require.NoError(t, err)
	assert.Equal(t, "ws-feat", tree.ResolveWorkspace(moved, "c-follow", f), "a chat filed away keeps its own")
}
