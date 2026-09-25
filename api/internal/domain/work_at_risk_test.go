package domain_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestWorkAtRisk_LostOnlyWhenSomethingWouldBe(t *testing.T) {
	assert.False(t, domain.WorkAtRisk{Branch: "b"}.Lost())
	assert.True(t, domain.WorkAtRisk{UncommittedFiles: 1}.Lost())
	assert.True(t, domain.WorkAtRisk{UnmergedCommits: 1}.Lost())
}

func TestWorkAtRiskError_NamesEveryBranchAndUnwrapsToTheSentinel(t *testing.T) {
	err := fmt.Errorf("delete: %w", &domain.WorkAtRiskError{Workspaces: []domain.WorkAtRisk{
		{Branch: "a", UncommittedFiles: 3},
		{Branch: "b", UnmergedCommits: 2},
	}})

	require.ErrorIs(t, err, domain.ErrWorkAtRisk)
	assert.Contains(t, err.Error(), "a (3 uncommitted files, 0 unmerged commits); b (0 uncommitted files, 2 unmerged commits)")
}
