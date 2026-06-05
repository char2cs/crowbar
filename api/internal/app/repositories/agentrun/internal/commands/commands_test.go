package commands

import (
	"errors"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestCreateAgentRun_EmitsPending(t *testing.T) {
	run := CreateAgentRun{ID: "a1", WsID: "w1", ChatID: "c1", Now: time.Unix(1, 0)}.EmitEvent(nil)
	assert.Equal(t, domain.AgentRunStatusPending, run.Status)
}

func TestCreateAgentRun_Validate_RejectsExisting(t *testing.T) {
	err := CreateAgentRun{ID: "a1", WsID: "w1", ChatID: "c1"}.Validate(&domain.AgentRun{ID: "a1"})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestMarkRunning_RequiresPending(t *testing.T) {
	err := MarkAgentRunRunning{ID: "a1"}.Validate(&domain.AgentRun{Status: domain.AgentRunStatusRunning})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
	run := MarkAgentRunRunning{ID: "a1"}.EmitEvent(&domain.AgentRun{Status: domain.AgentRunStatusPending})
	assert.Equal(t, domain.AgentRunStatusRunning, run.Status)
}

func TestComplete_RequiresRunning(t *testing.T) {
	err := CompleteAgentRun{ID: "a1"}.Validate(&domain.AgentRun{Status: domain.AgentRunStatusPending})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
	run := CompleteAgentRun{ID: "a1"}.EmitEvent(&domain.AgentRun{Status: domain.AgentRunStatusRunning})
	assert.Equal(t, domain.AgentRunStatusDone, run.Status)
}

func TestFail_RequiresRunning_ThenErrors(t *testing.T) {
	err := FailAgentRun{ID: "a1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
	run := FailAgentRun{ID: "a1"}.EmitEvent(&domain.AgentRun{Status: domain.AgentRunStatusRunning})
	assert.Equal(t, domain.AgentRunStatusError, run.Status)
}

func TestAgentRun_CommandMetadata(t *testing.T) {
	assert.Equal(t, "a1", CreateAgentRun{ID: "a1"}.AggregateID())
	assert.False(t, CreateAgentRun{ID: "a1"}.ShouldSnapshot())
	assert.Contains(t, FailAgentRun{ID: "a1"}.EventName(), "error")
	assert.Contains(t, MarkAgentRunRunning{ID: "a1"}.EventName(), "running")
	assert.Contains(t, CompleteAgentRun{ID: "a1"}.EventName(), "done")
}

func TestAgentRun_AllMetadata(t *testing.T) {
	assert.Equal(t, "a1", MarkAgentRunRunning{ID: "a1"}.AggregateID())
	assert.False(t, MarkAgentRunRunning{ID: "a1"}.ShouldSnapshot())

	assert.Equal(t, "a1", CompleteAgentRun{ID: "a1"}.AggregateID())
	assert.False(t, CompleteAgentRun{ID: "a1"}.ShouldSnapshot())

	assert.Equal(t, "a1", FailAgentRun{ID: "a1"}.AggregateID())
	assert.False(t, FailAgentRun{ID: "a1"}.ShouldSnapshot())

	assert.Contains(t, CreateAgentRun{ID: "a1"}.EventName(), "created")
}

func TestCreateAgentRun_Validate_RejectsMissingIDs(t *testing.T) {
	err := CreateAgentRun{ID: "a1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestCreateAgentRun_Validate_AcceptsValidNew(t *testing.T) {
	err := CreateAgentRun{ID: "a1", WsID: "w1", ChatID: "c1"}.Validate(nil)
	assert.NoError(t, err)
}

func TestMarkRunning_Validate_RejectsNil(t *testing.T) {
	err := MarkAgentRunRunning{ID: "a1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestMarkRunning_Validate_AcceptsPending(t *testing.T) {
	err := MarkAgentRunRunning{ID: "a1"}.Validate(&domain.AgentRun{Status: domain.AgentRunStatusPending})
	assert.NoError(t, err)
}

func TestComplete_Validate_RejectsNil(t *testing.T) {
	err := CompleteAgentRun{ID: "a1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestComplete_Validate_AcceptsRunning(t *testing.T) {
	err := CompleteAgentRun{ID: "a1"}.Validate(&domain.AgentRun{Status: domain.AgentRunStatusRunning})
	assert.NoError(t, err)
}

func TestFail_Validate_RejectsNotRunning(t *testing.T) {
	err := FailAgentRun{ID: "a1"}.Validate(&domain.AgentRun{Status: domain.AgentRunStatusPending})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestFail_Validate_AcceptsRunning(t *testing.T) {
	err := FailAgentRun{ID: "a1"}.Validate(&domain.AgentRun{Status: domain.AgentRunStatusRunning})
	assert.NoError(t, err)
}
