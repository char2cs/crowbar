package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

var errFake = errors.New("fake error")

type errDirectory struct{}

func (errDirectory) Upsert(context.Context, domain.Workspace) error { return nil }
func (errDirectory) Delete(context.Context, string) error           { return nil }
func (errDirectory) ListByRepo(context.Context, string, string) ([]domain.Workspace, error) {
	return nil, errFake
}
func (errDirectory) Rebuild(context.Context, []domain.Workspace) error { return nil }

func TestListWorkspacesInRepo_DirectoryErrorPropagates(t *testing.T) {
	c := &Container{directory: errDirectory{}, inflight: map[string]int{}}

	rows, err := c.ListWorkspacesInRepo(context.Background(), "p1", "r1")

	require.Error(t, err)
	assert.Nil(t, rows)
}
