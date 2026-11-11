package tree_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	repotree "github.com/char2cs/crowbar/api/internal/app/repositories/tree"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// openTree builds a Tree instance over SQLite stores in dir; a second call on
// the same dir is the same home after a restart.
func openTree(t *testing.T, dir string) asynx.Asynx[tree.Tree] {
	t.Helper()
	es, err := eventsqlite.NewEventStore(filepath.Join(dir, "tree.db"))
	require.NoError(t, err)
	ss, err := eventsqlite.NewSnapshotStore(filepath.Join(dir, "tree_snapshots.db"))
	require.NoError(t, err)
	ax, err := asynx.New[tree.Tree]().WithEventStore(es).WithSnapshotStore(ss).Build()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ax.Shutdown(context.Background())) })
	return ax
}

func newRepo(t *testing.T) *repotree.Repository {
	t.Helper()
	return repotreeOver(t, t.TempDir())
}

func repotreeOver(t *testing.T, dir string) *repotree.Repository {
	t.Helper()
	return repotree.New(openTree(t, dir))
}
