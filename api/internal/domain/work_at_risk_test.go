package domain_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func TestWorkAtRisk_LostOnlyWhenSomethingWouldBe(t *testing.T) {
	assert.False(t, domain.WorkAtRisk{Branch: "b"}.Lost())
	assert.True(t, domain.WorkAtRisk{UncommittedFiles: 1}.Lost())
	assert.True(t, domain.WorkAtRisk{UnmergedCommits: 1}.Lost())
}

func TestWorkAtRisked_NamesEveryBranchAndCarriesTheList(t *testing.T) {
	risks := []domain.WorkAtRisk{
		{Branch: "a", UncommittedFiles: 3},
		{Branch: "b", UnmergedCommits: 2},
	}
	err := fmt.Errorf("delete: %w", domain.WorkAtRisked(risks))

	require.ErrorIs(t, err, domain.ErrWorkAtRisk)
	assert.Equal(t, problem.WorkAtRisk, problem.CodeOf(err))
	assert.Equal(t, map[string]any{"workAtRisk": risks}, problem.DataOf(err))
	assert.Equal(t, "delete: delete would destroy work that exists nowhere else: "+
		"a (3 uncommitted files, 0 unmerged commits); b (0 uncommitted files, 2 unmerged commits)", err.Error())
}
