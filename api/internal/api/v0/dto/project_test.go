package dto_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestProjectDTOFrom(
	t *testing.T,
) {
	now := time.Unix(42, 0).UTC()
	got := dto.ProjectDTOFrom(domain.Project{
		ID:           "p1",
		Name:         "alpha",
		Path:         "/a",
		LastActivity: now,
	})
	assert.Equal(t, "p1", got.ID)
	assert.Equal(t, "alpha", got.Name)
	assert.Equal(t, "/a", got.Path)
	assert.Equal(t, now, got.LastActivity)
}

func TestProjectDTOListEmptyNonNil(
	t *testing.T,
) {
	got := dto.ProjectDTOList(nil)
	require.NotNil(t, got)
	assert.Len(t, got, 0)
}

func TestProjectDTOList(
	t *testing.T,
) {
	got := dto.ProjectDTOList([]domain.Project{
		{ID: "p1"},
		{ID: "p2"},
	})
	require.Len(t, got, 2)
	assert.Equal(t, "p1", got[0].ID)
	assert.Equal(t, "p2", got[1].ID)
}

// The list is ordered by the converter itself, which is what makes the REST list
// and the WS snapshot — the two callers — incapable of disagreeing about the
// sidebar order.
func TestProjectDTOList_OrdersByIndexThenID(
	t *testing.T,
) {
	got := dto.ProjectDTOList([]domain.Project{
		{ID: "c", Order: 2},
		{ID: "b"},
		{ID: "a"},
	})
	require.Len(t, got, 3)
	assert.Equal(t, []string{"a", "b", "c"}, []string{got[0].ID, got[1].ID, got[2].ID})
	assert.Equal(t, 2, got[2].Order, "the order reaches the wire")
}
