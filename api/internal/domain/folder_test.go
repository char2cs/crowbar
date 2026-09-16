package domain_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func newFolderStore(
	t *testing.T,
) (context.Context, store.ScopedStore[domain.Folder, string]) {
	t.Helper()
	s, err := sqlite.New[domain.Folder, string](":memory:")
	require.NoError(t, err)
	return context.Background(), s
}

func TestFolder_CreateAndGet(t *testing.T) {
	ctx, s := newFolderStore(t)
	require.NoError(t, s.Save(ctx, domain.Folder{ID: "f1", Name: "Work", RepoID: "r1"}))

	got, err := s.FindByKey(ctx, "f1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Work", got.Name)
	assert.Equal(t, "r1", got.RepoID)
}

func TestFolder_Rename(t *testing.T) {
	ctx, s := newFolderStore(t)
	require.NoError(t, s.Save(ctx, domain.Folder{ID: "f1", Name: "Work", RepoID: "r1"}))

	require.NoError(t, s.Save(ctx, domain.Folder{ID: "f1", Name: "Personal", RepoID: "r1"}))

	got, err := s.FindByKey(ctx, "f1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Personal", got.Name)
}

func TestFolder_ListByRepo(t *testing.T) {
	ctx, s := newFolderStore(t)
	require.NoError(t, s.Save(ctx, domain.Folder{ID: "f1", Name: "a", RepoID: "r1"}))
	require.NoError(t, s.Save(ctx, domain.Folder{ID: "f2", Name: "b", RepoID: "r1"}))
	require.NoError(t, s.Save(ctx, domain.Folder{ID: "f3", Name: "c", RepoID: "r2"}))

	got, err := s.FindWhere(ctx, domain.Folder{RepoID: "r1"})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.ElementsMatch(t, []string{"f1", "f2"}, []string{got[0].ID, got[1].ID})
}

// A blank RepoID is "don't care" under FindWhere's own zero-field semantics
// (store.ScopedStore's doc comment), so a project-home listing (RepoID == "")
// is drawn in memory over FindAll, never expressed as a FindWhere match.
func TestFolder_ListProjectHome(t *testing.T) {
	ctx, s := newFolderStore(t)
	require.NoError(t, s.Save(ctx, domain.Folder{ID: "f1", Name: "home-a", RepoID: ""}))
	require.NoError(t, s.Save(ctx, domain.Folder{ID: "f2", Name: "home-b", RepoID: ""}))
	require.NoError(t, s.Save(ctx, domain.Folder{ID: "f3", Name: "in-repo", RepoID: "r1"}))

	all, err := s.FindAll(ctx)
	require.NoError(t, err)

	var projectHome []domain.Folder
	for _, f := range all {
		if f.RepoID == "" {
			projectHome = append(projectHome, f)
		}
	}
	require.Len(t, projectHome, 2)
	assert.ElementsMatch(t, []string{"f1", "f2"}, []string{projectHome[0].ID, projectHome[1].ID})
}

func TestFolder_Delete(t *testing.T) {
	ctx, s := newFolderStore(t)
	require.NoError(t, s.Save(ctx, domain.Folder{ID: "f1", Name: "Work", RepoID: "r1"}))

	require.NoError(t, s.Delete(ctx, "f1"))

	got, err := s.FindByKey(ctx, "f1")
	require.NoError(t, err)
	assert.Nil(t, got)
}
