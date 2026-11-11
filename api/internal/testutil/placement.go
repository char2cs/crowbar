package testutil

import (
	"context"
	"testing"

	"github.com/char2cs/asynx"
	asynxstore "github.com/char2cs/asynx/store"

	repotree "github.com/char2cs/crowbar/api/internal/app/repositories/tree"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// Placements is the real placement store over an in-memory Tree, for a test
// that builds a chat repository without the app. It names no project, so every
// row is kept in the Unplaced tree, which is read like any other.
func Placements(tb testing.TB) *repotree.Ports {
	tb.Helper()
	ax, err := asynx.New[tree.Tree]().
		WithEventStore(asynxstore.New()).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		Build()
	if err != nil {
		tb.Fatalf("testutil: tree asynx: %v", err)
	}
	tb.Cleanup(func() {
		if err := ax.Shutdown(context.Background()); err != nil {
			tb.Errorf("testutil: tree asynx shutdown: %v", err)
		}
	})
	nowhere := func(context.Context, string) (string, error) { return "", problem.ErrNotFound }
	locate := repotree.NewLocator(repotree.LocatorFacts{
		ChatWorkspace: nowhere, WorkspaceProject: nowhere, RepoProject: nowhere,
	})
	p, err := repotree.NewPorts(context.Background(), repotree.New(ax), locate, nil)
	if err != nil {
		tb.Fatalf("testutil: placement ports: %v", err)
	}
	return p
}
