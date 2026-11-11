package descriptor_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/inbound"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func TestCodexDescriptor_CommandActionsGiveKindTargetAndExitCode(t *testing.T) {
	d := loadDescriptor(t, "codex")
	for _, tc := range []struct {
		event, file, kind, target string
		exit                      int
	}{
		{"tool_post", "item_completed.commandExecution.read.json", "read", "cat notes.txt", 0},
		{"tool_post", "item_completed.commandExecution.json", "execute", "ls -la", 0}, // a listing it ran, not a search
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

// A command names its process and directory when it starts, its output when it ends,
// and input written to the process later names the same call.
func TestCodexDescriptor_ACommandNamesItsProcessAndWhereItRan(t *testing.T) {
	d := loadDescriptor(t, "codex")

	started := parseRecorded(t, d, "tool_pre", "item_started.commandExecution.json", spec.ChannelAPI)
	expectEq(t, "ref", started.ShellRef, "10527")
	expectEq(t, "cwd", started.Cwd, "/tmp/codex_probe_ws")

	done := parseRecorded(t, d, "tool_post", "item_completed.commandExecution.json", spec.ChannelAPI)
	expectEq(t, "output", done.Output, "total 0\ndrwxr-xr-x@   2 char2cs  wheel    64 Sep  8 12:41 .\ndrwxrwxrwt  304 root     wheel  9728 Sep  8 12:41 ..\n")
	failed := parseRecorded(t, d, "tool_fail", "item_completed.commandExecutionFailed.json", spec.ChannelAPI)
	expectEq(t, "failed output", failed.Output != "", true)

	raw, err := os.ReadFile(fixtureRoot + "/codex/item_commandExecution_terminalInteraction.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var frame struct{ Params json.RawMessage }
	if err := json.Unmarshal(raw, &frame); err != nil {
		t.Fatal(err)
	}
	input := parseRaw(t, d, "terminal_input", frame.Params, spec.ChannelAPI)
	expectEq(t, "input names the call", input.ID, "call_PQbUMzNNrlEGDg3Y4zc7JwNW")
	expectEq(t, "input names the process", input.ShellRef, "10527")
}

// A failed turn is classified by the descriptor, from the status code or the named error it reports.
func TestCodexDescriptor_ATurnFailureCarriesItsNeutralKind(t *testing.T) {
	d := loadDescriptor(t, "codex")
	for name, tc := range map[string]struct{ info, want string }{
		"http status": {`{"httpConnectionFailed":{"httpStatusCode":401}}`, spec.FailureAuth},
		"rate status": {`{"httpConnectionFailed":{"httpStatusCode":429}}`, spec.FailureLimit},
		"named limit": {`"usageLimitExceeded"`, spec.FailureLimit},
		"named busy":  {`"serverOverloaded"`, spec.FailureOverloaded},
		"unmapped":    {`"badRequest"`, ""},
		"none":        {`null`, ""},
	} {
		raw := []byte(`{"threadId":"t","turn":{"id":"u","status":"failed","error":{"message":"m","codexErrorInfo":` + tc.info + `}}}`)
		ev, err := inbound.Parse(d, spec.HookTurnFailed, raw, spec.ChannelAPI)
		if err != nil || ev.Failure == nil {
			t.Fatalf("%s: failure %v, err %v", name, ev.Failure, err)
		}
		expectEq(t, name+" kind", ev.Failure.Kind, tc.want)
	}
}
