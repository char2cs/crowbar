package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestTerminalProfileDTOFrom_NormalisesCommands(t *testing.T) {
	got := dto.TerminalProfileDTOFrom(domain.TerminalProfile{
		ID:   "p1",
		Name: "bash",
	})
	assert.Equal(t, "p1", got.ID)
	assert.NotNil(t, got.StartupCommands)
	assert.Empty(t, got.StartupCommands)
}

func TestTerminalProfileDTOFrom_CopiesCommands(t *testing.T) {
	got := dto.TerminalProfileDTOFrom(domain.TerminalProfile{
		ID:              "p2",
		StartupCommands: []string{"echo a", "echo b"},
	})
	assert.Equal(t, []string{"echo a", "echo b"}, got.StartupCommands)
}

func TestTerminalProfileDTOList_EmptyNonNil(t *testing.T) {
	got := dto.TerminalProfileDTOList(nil)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestTerminalProfileDTOList_Converts(t *testing.T) {
	got := dto.TerminalProfileDTOList([]domain.TerminalProfile{
		{ID: "a"},
		{ID: "b"},
	})
	assert.Len(t, got, 2)
	assert.Equal(t, "a", got[0].ID)
	assert.Equal(t, "b", got[1].ID)
}
