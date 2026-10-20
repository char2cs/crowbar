package descriptor_test

import (
	"os"
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/inbound"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func TestClaudeDescriptor_MapsMCPServerToolAndTitle(t *testing.T) {
	d := loadDescriptor(t, "claude")
	for event, file := range map[string]string{"tool_pre": "PreToolUse.mcp.json", "tool_post": "PostToolUse.mcp.json"} {
		tool := parseRecorded(t, d, event, file, spec.ChannelHooks)
		expectEq(t, event+" server", tool.MCPServer, "echo")
		expectEq(t, event+" tool", tool.MCPTool, "shout")
		expectEq(t, event+" name", tool.Name, "mcp__echo__shout")
		expectEq(t, event+" kind", tool.Kind, "mcp")
	}
}

func TestClaudeDescriptor_MapsBashDescriptionAsTitle(t *testing.T) {
	d := loadDescriptor(t, "claude")
	const title = "List files in working directory with details"
	for _, tc := range []struct{ event, file, title, target, kind string }{
		{"tool_pre", "PreToolUse.Bash.json", title, "ls -la", "execute"},
		{"tool_post", "PostToolUse.Bash.json", title, "ls -la", "execute"},
		{"tool_pre", "PreToolUse.TaskUpdate.json", "", "1", "plan"},
	} {
		tool := parseRecorded(t, d, tc.event, tc.file, spec.ChannelHooks)
		expectEq(t, tc.file+" title", tool.Title, tc.title)
		expectEq(t, tc.file+" target", tool.Target, tc.target)
		expectEq(t, tc.file+" kind", tool.Kind, tc.kind)
		expectEq(t, tc.file+" server and tool", tool.MCPServer+tool.MCPTool, "")
	}
}

// bashFailure is the claude 2.1.302 PostToolUseFailure capture, scrubbed only; it cannot join the
// replay because no recorded subagent failure resolves subagent_id.
const bashFailure = `{
  "session_id": "6f1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d",
  "transcript_path": "/h/.claude/projects/-ws/6f1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d.jsonl",
  "cwd": "/ws",
  "prompt_id": "9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d",
  "permission_mode": "default",
  "effort": {
    "level": "medium"
  },
  "hook_event_name": "PostToolUseFailure",
  "tool_name": "Bash",
  "tool_input": {
    "command": "cat does-not-exist.txt",
    "description": "Attempt to print a missing file (expected to fail)"
  },
  "tool_use_id": "toolu_01DQe9RP5NHcSiLciwSXoWiC",
  "error": "Exit code 1\ncat: does-not-exist.txt: No such file or directory",
  "is_interrupt": false,
  "duration_ms": 30
}`

func TestClaudeDescriptor_ToolFailCarriesInterrupt(t *testing.T) {
	d := loadDescriptor(t, "claude")
	failed := parseRaw(t, d, "tool_fail", []byte(bashFailure), spec.ChannelHooks)
	expectEq(t, "title", failed.Title, "Attempt to print a missing file (expected to fail)")
	expectEq(t, "interrupted", failed.Interrupted, false)

	cut := strings.Replace(bashFailure, `"is_interrupt": false`, `"is_interrupt": true`, 1)
	expectEq(t, "interrupted", parseRaw(t, d, "tool_fail", []byte(cut), spec.ChannelHooks).Interrupted, true)
}

func TestClaudeDescriptor_PermissionNamesItsCallFromTheRecordedAsk(t *testing.T) {
	d := loadDescriptor(t, "claude")
	for file, want := range map[string]struct{ tool, target, server string }{
		"PermissionRequest.mcp.json":  {"mcp__echo__shout", "", "echo"},
		"PermissionRequest.Edit.json": {"Edit", "/ws/calc.py", ""},
	} {
		raw, err := os.ReadFile(fixtureRoot + "/claude/" + file)
		if err != nil {
			t.Fatal(err)
		}
		ev, err := inbound.Parse(d, "permission", raw, spec.ChannelHooks)
		if err != nil || ev.Choice == nil {
			t.Fatalf("%s: choice %v, err %v", file, ev.Choice, err)
		}
		expectEq(t, file+" tool", ev.Choice.ToolName, want.tool)
		expectEq(t, file+" target", ev.Choice.ToolTarget, want.target)
		expectEq(t, file+" server", ev.Choice.MCPServer, want.server)
	}
}
