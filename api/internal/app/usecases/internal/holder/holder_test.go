package holder_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/holder"
	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

type fakeEngine struct {
	pruned  []string
	entries []gitengine.WorktreeEntry
	listErr error
}

func (f *fakeEngine) WorktreePrune(_ context.Context, repoPath string) error {
	f.pruned = append(f.pruned, repoPath)
	return nil
}

func (f *fakeEngine) WorktreeList(_ context.Context, _ string) ([]gitengine.WorktreeEntry, error) {
	return f.entries, f.listErr
}

func TestResolve_PrunesFirst(t *testing.T) {
	e := &fakeEngine{}
	_, err := holder.Resolve(context.Background(), e, "/repo", "develop", "/crowbarhome")
	require.NoError(t, err)
	assert.Equal(t, []string{"/repo"}, e.pruned, "prune runs before listing (dead-reg case)")
}

func TestResolve_FreeWhenNoHolder(t *testing.T) {
	e := &fakeEngine{entries: []gitengine.WorktreeEntry{{Path: "/repo", Branch: "main"}}}
	out, err := holder.Resolve(context.Background(), e, "/repo", "develop", "/crowbarhome")
	require.NoError(t, err)
	assert.Equal(t, holder.Free, out.Kind)
	assert.Empty(t, out.HeldByPath)
}

func TestResolve_HeldByHome(t *testing.T) {
	e := &fakeEngine{entries: []gitengine.WorktreeEntry{{Path: "/repo", Branch: "develop"}}}
	out, err := holder.Resolve(context.Background(), e, "/repo", "develop", "/crowbarhome")
	require.NoError(t, err)
	assert.Equal(t, holder.HeldByHome, out.Kind)
	assert.Equal(t, "/repo", out.HeldByPath)
}

func TestResolve_HeldByManaged(t *testing.T) {
	e := &fakeEngine{entries: []gitengine.WorktreeEntry{
		{Path: "/crowbarhome/projects/p/r/workspaces/w/worktree", Branch: "develop"},
	}}
	out, err := holder.Resolve(context.Background(), e, "/repo", "develop", "/crowbarhome")
	require.NoError(t, err)
	assert.Equal(t, holder.HeldByManaged, out.Kind)
}

func TestResolve_HeldByExternal(t *testing.T) {
	e := &fakeEngine{entries: []gitengine.WorktreeEntry{
		{Path: "/somewhere/else", Branch: "develop"},
	}}
	out, err := holder.Resolve(context.Background(), e, "/repo", "develop", "/crowbarhome")
	require.NoError(t, err)
	assert.Equal(t, holder.HeldByExternal, out.Kind)
	assert.Equal(t, "/somewhere/else", out.HeldByPath)
}

func TestResolve_ListError(t *testing.T) {
	e := &fakeEngine{listErr: errors.New("boom")}
	_, err := holder.Resolve(context.Background(), e, "/repo", "develop", "/crowbarhome")
	require.Error(t, err)
}
