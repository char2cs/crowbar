package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// A finished step runs once per install; an unfinished one runs again.
func TestRunUpgrades_RunsEachStepUntilItFinishesAndNeverAgain(t *testing.T) {
	home := t.TempDir()
	done, flaky := 0, 0
	steps := []upgradeStep{
		{name: "done", run: func(context.Context) bool { done++; return true }},
		{name: "flaky", run: func(context.Context) bool { flaky++; return flaky > 1 }},
	}

	for range 3 {
		runUpgrades(context.Background(), home, steps)
	}

	assert.Equal(t, 1, done)
	assert.Equal(t, 2, flaky, "retried until it finished, then skipped")
}

type fakeHomeCreator struct {
	created []string
	err     error
}

func (f *fakeHomeCreator) CreateHome(_ context.Context, p domain.Project) (domain.Workspace, error) {
	f.created = append(f.created, p.ID)
	return domain.Workspace{}, f.err
}

// Only a live project with no live home gets one.
func TestReconcileProjectHomes_CreatesOnlyTheMissingHomes(t *testing.T) {
	c := &fakeHomeCreator{}
	complete := reconcileProjectHomes(context.Background(),
		[]domain.Project{{ID: "homed"}, {ID: "bare"}, {ID: "going", Deleting: true}, {ID: "tombstoned"}},
		[]domain.Workspace{
			{ID: "h1", ProjectID: "homed", Kind: domain.WorkspaceKindHome},
			{ID: "h2", ProjectID: "tombstoned", Kind: domain.WorkspaceKindHome, Status: domain.WorkspaceStatusDeleted},
			{ID: "w", ProjectID: "bare", Kind: domain.WorkspaceKindGit},
		}, c)

	assert.True(t, complete)
	assert.Equal(t, []string{"bare", "tombstoned"}, c.created)
}

func TestReconcileProjectHomes_AFailureLeavesTheStepUnfinished(t *testing.T) {
	c := &fakeHomeCreator{err: errors.New("boom")}
	assert.False(t, reconcileProjectHomes(context.Background(), []domain.Project{{ID: "bare"}}, nil, c))
}

func TestDropRetiredTable_DropsItAndToleratesItsAbsence(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE workspace_paths (id TEXT)").Error)

	require.True(t, dropRetiredTable(context.Background(), db, "workspace_paths"))
	require.True(t, dropRetiredTable(context.Background(), db, "workspace_paths"))
	assert.False(t, db.Migrator().HasTable("workspace_paths"))
}
