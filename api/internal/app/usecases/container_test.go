package usecases

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/repositories"
)

func TestNew_ReturnsContainer(t *testing.T) {
	repos := &repositories.Container{}
	container := New(repos, builtinLoader())
	assert.NotNil(t, container)
	assert.NotNil(t, container.Project)
	assert.NotNil(t, container.Repository)
	assert.NotNil(t, container.Task)
	assert.NotNil(t, container.AgentRun)
	assert.NotNil(t, container.KanbanItem)
	assert.NotNil(t, container.ReviewThread)
	assert.NotNil(t, container.Conversation)
	assert.NotNil(t, container.Git)
	assert.NotNil(t, container.Health)
}
