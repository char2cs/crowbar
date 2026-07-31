package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestFolderDTOFrom(t *testing.T) {
	got := dto.FolderDTOFrom(domain.Folder{
		ID: "f1", RepoID: "r1", ProjectID: "p1", ParentID: "w1", Name: "spikes", Order: 3,
	})

	assert.Equal(t, "f1", got.ID)
	assert.Equal(t, "r1", got.RepoID)
	assert.Equal(t, "p1", got.ProjectID)
	assert.Equal(t, "w1", got.ParentID)
	assert.Equal(t, "spikes", got.Name)
	assert.Equal(t, 3, got.Order)
	assert.Empty(t, got.Status, "a read-path DTO carries no tombstone marker")
}

func TestFolderDTOListEmptyNonNil(t *testing.T) {
	got := dto.FolderDTOList(nil)

	require.NotNil(t, got, "the envelope must carry [] rather than null")
	assert.Empty(t, got)
}

// The list is ordered by the converter itself, which is what makes the REST list
// and the WS snapshot — the two callers — incapable of disagreeing.
func TestFolderDTOList_OrdersByIndexThenID(t *testing.T) {
	got := dto.FolderDTOList([]domain.Folder{
		{ID: "c", Order: 2},
		{ID: "b", Order: 0},
		{ID: "a", Order: 0},
	})

	require.Len(t, got, 3)
	assert.Equal(t, []string{"a", "b", "c"}, []string{got[0].ID, got[1].ID, got[2].ID})
}

// A row with no parent must serialise without the key, so a client can tell "at
// the repo root" from "filed somewhere" without a sentinel.
func TestFolderDTO_OmitsAnEmptyParent(t *testing.T) {
	raw, err := json.Marshal(dto.FolderDTOFrom(domain.Folder{ID: "f1", Name: "spikes"}))
	require.NoError(t, err)

	assert.NotContains(t, string(raw), `"parentId"`)
	assert.Contains(t, string(raw), `"order":0`, "order is always present, even at zero")
}
