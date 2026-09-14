package commands_test

import (
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// A subagent's own tool call belongs to ITS nested conversation, never the
// chat's top-level turn — InvokeSubagentTool must not open (or touch) a
// top-level turn the way InvokeTool's ensureTurn does, or a chat whose own
// turn already closed would grow a ghost turn the instant its subagent's
// child thread ran a tool. See turn/ingest.go's nested-session routing.
func TestInvokeSubagentTool_RecordsTheCallWithoutTouchingTheTopLevelTurn(t *testing.T) {
	c := commands.InvokeSubagentTool{
		ChatID: chat, SubagentID: "a1", ToolID: "tool-1", Name: "Bash", Target: "ls",
		RequestRef: "sha256:abc", Now: now,
	}
	require.NoError(t, c.Validate(nil))
	got := c.EmitEvent(nil)

	require.Contains(t, got.Tools, "tool-1")
	assert.Equal(t, "a1", got.Tools["tool-1"].SubagentID)
	assert.Empty(t, got.Tools["tool-1"].TurnID, "a nested tool call has no top-level turn")
	assert.Equal(t, domain.ToolStatusRunning, got.Tools["tool-1"].Status)
	assert.Nil(t, got.Turn, "must not open a top-level turn as a side effect")
	require.NotNil(t, got.Last.Tool)
	assert.Equal(t, domain.DeltaOpen, got.Last.Phase)
}

func TestInvokeSubagentTool_RejectsTheUnusableCases(t *testing.T) {
	testCases := []struct {
		name string
		cmd  commands.InvokeSubagentTool
	}{
		{"no chat", commands.InvokeSubagentTool{SubagentID: "a1", ToolID: "t"}},
		{"no subagent id", commands.InvokeSubagentTool{ChatID: chat, ToolID: "t"}},
		{"no tool id", commands.InvokeSubagentTool{ChatID: chat, SubagentID: "a1"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.ErrorIs(t, tc.cmd.Validate(nil), asynxModels.ErrValidation)
		})
	}
}

func TestCompleteSubagentTool_ClosesTheCallAndCarriesTheInvocationForward(t *testing.T) {
	invoked := commands.InvokeSubagentTool{
		ChatID: chat, SubagentID: "a1", ToolID: "tool-1", Name: "Bash", Target: "ls",
		RequestRef: "sha256:req", Now: now,
	}.EmitEvent(nil)

	c := commands.CompleteSubagentTool{
		ChatID: chat, SubagentID: "a1", ToolID: "tool-1", ResultRef: "sha256:res",
		Status: domain.ToolStatusOK, DurationMS: 80, Now: now.Add(time.Second),
	}
	require.NoError(t, c.Validate(&invoked))
	got := c.EmitEvent(&invoked)

	assert.NotContains(t, got.Tools, "tool-1", "a completed call is no longer in flight")
	require.NotNil(t, got.Last.Tool)
	assert.Equal(t, "a1", got.Last.Tool.SubagentID)
	assert.Equal(t, "Bash", got.Last.Tool.Name)
	assert.Equal(t, "sha256:res", got.Last.Tool.ResultRef)
	assert.Equal(t, 80, got.Last.Tool.DurationMS)
	require.NotNil(t, got.Last.Tool.EndedAt)
}

func TestCompleteSubagentTool_ForAnUnseenCallStillLeavesALegibleRecord(t *testing.T) {
	got := commands.CompleteSubagentTool{
		ChatID: chat, SubagentID: "a1", ToolID: "t", Name: "Read", Status: domain.ToolStatusError, Now: now,
	}.EmitEvent(nil)

	require.NotNil(t, got.Last.Tool)
	assert.Equal(t, "a1", got.Last.Tool.SubagentID)
	assert.Equal(t, "Read", got.Last.Tool.Name)
	assert.Equal(t, domain.ToolStatusError, got.Last.Tool.Status)
}

func TestCompleteSubagentTool_RejectsTheUnusableCases(t *testing.T) {
	testCases := []struct {
		name string
		cmd  commands.CompleteSubagentTool
	}{
		{"no chat", commands.CompleteSubagentTool{SubagentID: "a1", ToolID: "t"}},
		{"no subagent id", commands.CompleteSubagentTool{ChatID: chat, ToolID: "t"}},
		{"no tool id", commands.CompleteSubagentTool{ChatID: chat, SubagentID: "a1"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.ErrorIs(t, tc.cmd.Validate(nil), asynxModels.ErrValidation)
		})
	}
}
