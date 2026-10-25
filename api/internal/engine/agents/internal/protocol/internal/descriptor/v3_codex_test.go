package descriptor_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func TestCodexDescriptor_CommandActionsGiveKindTargetAndExitCode(t *testing.T) {
	d := loadDescriptor(t, "codex")
	for _, tc := range []struct {
		event, file, kind, target string
		exit                      int
	}{
		{"tool_post", "item_completed.commandExecution.read.json", "read", "cat notes.txt", 0},
		{"tool_post", "item_completed.commandExecution.json", "search", "ls -la", 0},
		{"tool_fail", "item_completed.commandExecutionFailed.json", "read", "cat does-not-exist.txt", 1},
	} {
		tool := parseRecorded(t, d, tc.event, tc.file, spec.ChannelAPI)
		expectEq(t, tc.file+" kind", tool.Kind, tc.kind)
		expectEq(t, tc.file+" target", tool.Target, tc.target)
		if tool.ExitCode == nil || *tool.ExitCode != tc.exit {
			t.Errorf("%s exit code = %v, want %d", tc.file, tool.ExitCode, tc.exit)
		}
	}
}

func TestCodexDescriptor_MapsMCPServerToolAndSpawnTitle(t *testing.T) {
	d := loadDescriptor(t, "codex")
	api := parseRecorded(t, d, "tool_pre", "item_started.mcpToolCall.json", spec.ChannelAPI)
	expectEq(t, "api server", api.MCPServer, "echo")
	expectEq(t, "api tool", api.MCPTool, "shout")
	expectEq(t, "api kind", api.Kind, "mcp")

	hook := parseRecorded(t, d, "tool_pre", "PreToolUse.mcp.json", spec.ChannelHooks)
	expectEq(t, "hooks tool", hook.MCPTool, "shout")
	expectEq(t, "hooks server", hook.MCPServer, "")
	expectEq(t, "hooks kind", hook.Kind, "mcp")

	spawn := parseRecorded(t, d, "tool_pre", "PreToolUse.spawnAgent.json", spec.ChannelHooks)
	expectEq(t, "hooks title", spawn.Title, "Run exactly: wc -l notes.txt. Return the command output.")
	bash := parseRecorded(t, d, "tool_pre", "PreToolUse.json", spec.ChannelHooks)
	expectEq(t, "hooks target", bash.Target, "cat notes.txt")
	expectEq(t, "hooks kind of a shell call", bash.Kind, "")
}

func TestCodexDescriptor_ApplyPatchTargetIsItsFirstFilePath(t *testing.T) {
	d := loadDescriptor(t, "codex")
	patch := parseRecorded(t, d, "tool_pre", "PreToolUse.applyPatch.json", spec.ChannelHooks)

	expectEq(t, "target", patch.Target, "/ws/calc.py")
}

// A hooks call carries its input, so the ask it raises binds to it by that.
func TestCodexDescriptor_AHooksToolCallCarriesItsInput(t *testing.T) {
	d := loadDescriptor(t, "codex")
	for _, file := range []string{"PreToolUse.json", "PreToolUse.mcp.json", "PreToolUse.spawnAgent.json", "PreToolUse.applyPatch.json"} {
		raw, err := os.ReadFile(fixtureRoot + "/codex/" + file)
		if err != nil {
			t.Fatal(err)
		}
		var payload struct {
			ToolInput json.RawMessage `json:"tool_input"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		var want, got any
		tool := parseRecorded(t, d, "tool_pre", file, spec.ChannelHooks)
		if json.Unmarshal(payload.ToolInput, &want) != nil || json.Unmarshal(tool.Input, &got) != nil || !reflect.DeepEqual(want, got) {
			t.Errorf("%s: input = %s, want %s", file, tool.Input, payload.ToolInput)
		}
	}
}
