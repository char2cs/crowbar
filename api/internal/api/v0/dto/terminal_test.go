package dto_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

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
