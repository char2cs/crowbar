package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/config"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agent/internal/session"
	"github.com/char2cs/crowbar/api/internal/engine/agent/internal/types"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
)

type mockHub struct{}

func (mockHub) RegisterSession(
	_ string,
) (<-chan string, func(types.ChatFrame), func()) {
	ch := make(chan string)
	return ch, func(types.ChatFrame) {}, func() {}
}

func TestRun_nilAgent_returnsError(
	t *testing.T,
) {
	cfg := config.IntelligenceConfig{
		Low: "claude-haiku-4-5-20251001",
	}
	rt := session.New(cfg, func(id string) string { return t.TempDir() }, mockHub{}, "", "")

	state := flow.StateDefinition{Name: "human_review"}
	err := rt.Run(
		context.Background(),
		domain.AgentRun{ID: "run-1"},
		domain.Task{ID: "task-1"},
		state,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no agent definition")
}

func TestRun_unknownIntelligence_returnsError(
	t *testing.T,
) {
	cfg := config.IntelligenceConfig{}
	rt := session.New(cfg, func(id string) string { return t.TempDir() }, mockHub{}, "", "")

	state := flow.StateDefinition{
		Name:  "coding",
		Agent: &flow.AgentDef{Intelligence: flow.IntelligenceLow},
	}
	err := rt.Run(
		context.Background(),
		domain.AgentRun{ID: "run-2"},
		domain.Task{ID: "task-2"},
		state,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve agent")
}
