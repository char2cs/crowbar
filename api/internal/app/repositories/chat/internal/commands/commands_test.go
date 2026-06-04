package commands

import (
	"errors"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestCreateChat_EmitsIdle(t *testing.T) {
	chat := CreateChat{ID: "c1", WsID: "w1", Now: time.Unix(1, 0)}.EmitEvent(nil)
	assert.Equal(t, domain.ChatStatusIdle, chat.Status)
}

func TestCreateChat_Validate_RejectsExisting(t *testing.T) {
	err := CreateChat{ID: "c1", WsID: "w1"}.Validate(&domain.Chat{ID: "c1"})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestResetIdle_FromAgentRunning(t *testing.T) {
	chat := ResetChatIdle{ID: "c1"}.EmitEvent(&domain.Chat{Status: domain.ChatStatusAgentRunning})
	assert.Equal(t, domain.ChatStatusIdle, chat.Status)
}

func TestResetIdle_IdempotentFromIdle(t *testing.T) {
	chat := ResetChatIdle{ID: "c1"}.EmitEvent(&domain.Chat{Status: domain.ChatStatusIdle})
	assert.Equal(t, domain.ChatStatusIdle, chat.Status)
}

func TestResetIdle_Validate_RejectsMissing(t *testing.T) {
	err := ResetChatIdle{ID: "c1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestChat_AllMetadata(t *testing.T) {
	create := CreateChat{ID: "c1"}
	assert.Equal(t, "c1", create.AggregateID())
	assert.Contains(t, create.EventName(), "chat.created")
	assert.False(t, create.ShouldSnapshot())

	reset := ResetChatIdle{ID: "c1"}
	assert.Equal(t, "c1", reset.AggregateID())
	assert.Contains(t, reset.EventName(), "idle_reset")
	assert.False(t, reset.ShouldSnapshot())
}

func TestCreateChat_Validate_RejectsMissingIDs(t *testing.T) {
	err := CreateChat{ID: "c1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestCreateChat_Validate_AcceptsValidNew(t *testing.T) {
	err := CreateChat{ID: "c1", WsID: "w1"}.Validate(nil)
	assert.NoError(t, err)
}

func TestResetIdle_Validate_AcceptsExisting(t *testing.T) {
	err := ResetChatIdle{ID: "c1"}.Validate(&domain.Chat{ID: "c1"})
	assert.NoError(t, err)
}
