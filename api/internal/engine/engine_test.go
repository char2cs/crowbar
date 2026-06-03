package engine_test

import (
	"context"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine"
)

func TestEngineContainerNew_NilHub(t *testing.T) {
	c, err := engine.New(context.Background(), nil)
	require.NoError(t, err)
	assert.NotNil(t, c)
	assert.Nil(t, c.AgentRuntime)
	assert.NotNil(t, c.FlowLoader)
}

type mockRuntime struct{}

func (m *mockRuntime) Run(_ context.Context, _ domain.AgentRun, _ domain.Task, _ flow.StateDefinition) error {
	return nil
}

func TestEngineContainerNew_WithAgentRuntime(t *testing.T) {
	rt := &mockRuntime{}
	c, err := engine.New(context.Background(), nil, engine.WithAgentRuntime(rt))
	require.NoError(t, err)
	assert.NotNil(t, c)
	assert.Equal(t, rt, c.AgentRuntime)
}

func TestEngineContainerNew_WithHomeDir_NilHub(t *testing.T) {
	c, err := engine.New(context.Background(), nil, engine.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	assert.NotNil(t, c)
	assert.Nil(t, c.AgentRuntime)
}

type mockHub struct{}

func (m *mockHub) RegisterSession(_ string) (<-chan string, func(agent.ChatFrame), func()) {
	ch := make(chan string)
	return ch, func(agent.ChatFrame) {}, func() {}
}

func TestEngineContainerNew_WithHub(t *testing.T) {
	c, err := engine.New(context.Background(), &mockHub{})
	require.NoError(t, err)
	assert.NotNil(t, c)
	assert.NotNil(t, c.AgentRuntime)
}

func TestEngineContainerNew_WithHubAndHomeDir(t *testing.T) {
	c, err := engine.New(context.Background(), &mockHub{}, engine.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	assert.NotNil(t, c)
	assert.NotNil(t, c.AgentRuntime)
}
