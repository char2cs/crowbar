package dto_test

import (
	"testing"
	"time"

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

func TestTerminalSessionDTOFrom_PopulatesChatID(t *testing.T) {
	created := time.Now().UTC()
	got := dto.TerminalSessionDTOFrom(
		"s1",
		"c1",
		"prof1",
		"active",
		created,
	)

	assert.Equal(t, "s1", got.ID)
	assert.Equal(t, "c1", got.ChatID)
	assert.Equal(t, "prof1", got.ProfileID)
	assert.Equal(t, "active", got.Status)
	assert.Equal(t, created, got.CreatedAt)
	assert.Nil(t, got.EndedAt)
}

func TestTerminalSessionDTOFrom_ActiveAndEndedStatus(t *testing.T) {
	active := dto.TerminalSessionDTOFrom(
		"s1",
		"c1",
		"",
		"active",
		time.Now().UTC(),
	)
	assert.Equal(t, "active", active.Status)
	assert.Empty(t, active.ProfileID)

	ended := dto.TerminalSessionDTOFrom(
		"s1",
		"c1",
		"",
		"ended",
		time.Now().UTC(),
	)
	assert.Equal(t, "ended", ended.Status)
}

func TestTerminalSessionDTOList_NonNilEmptySlice(t *testing.T) {
	created := time.Now().UTC()
	got := dto.TerminalSessionDTOList(
		nil,
		"c1",
		"prof1",
		"active",
		created,
	)
	assert.NotNil(t, got)
	assert.Empty(t, got)

	got = dto.TerminalSessionDTOList(
		[]string{"s1", "s2"},
		"c1",
		"prof1",
		"active",
		created,
	)
	assert.Len(t, got, 2)
	assert.Equal(t, "s1", got[0].ID)
	assert.Equal(t, "s2", got[1].ID)
	assert.Equal(t, "c1", got[0].ChatID)
	assert.Equal(t, "active", got[1].Status)
}
