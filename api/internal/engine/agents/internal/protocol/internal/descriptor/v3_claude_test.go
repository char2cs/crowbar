package descriptor_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
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
// replay because no recorded subagent failure resolves scope_id.
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

// A subagent's hand-back is its result, never the user's prompt; its prompt
// id names the parent's turn that answers it.
func TestInjectedSubagentResultIsNotAUserPrompt(t *testing.T) {
	d := loadDescriptor(t, "claude")
	raw, err := os.ReadFile(fixtureRoot + "/claude/UserPromptSubmit.AgentMessage.json")
	if err != nil {
		t.Fatal(err)
	}
	ev, err := inbound.Parse(d, spec.HookUserPrompt, raw, spec.ChannelHooks)
	if err != nil || ev.Subagent == nil {
		t.Fatalf("subagent %v, err %v", ev.Subagent, err)
	}
	expectEq(t, "kind", ev.Kind, spec.HookSubagentResult)
	expectEq(t, "scope", ev.Subagent.ID, "a3edc502687d15840")
	expectEq(t, "turn", ev.TurnID, "bfbceb05-8583-4143-8108-046be645e2c6")
	expectEq(t, "result", ev.Subagent.Result, "a.txt contains \"alpha\" (one line), and `echo hi` printed \"hi\".")
	expectEq(t, "no prompt", ev.Message, "")
}

func TestClaudeDescriptor_ABackgroundAgentCallNamesTheScopeItSpawned(t *testing.T) {
	d := loadDescriptor(t, "claude")
	spawn := parseRecorded(t, d, "tool_post", "PostToolUse.Agent.json", spec.ChannelHooks)
	expectEq(t, "kind", spawn.Kind, "subagent")
	expectEq(t, "spawns", spawn.SpawnsScope, "a3edc502687d15840")
}

// A TaskUpdate's completion is one plan step changed alone.
func TestClaudeDescriptor_ATaskUpdateIsAPlanStep(t *testing.T) {
	d := loadDescriptor(t, "claude")
	ev, err := inbound.Parse(d, spec.HookPlanStep, []byte(`{"session_id":"s","transcript_path":"/t",
		"hook_event_name":"PostToolUse","tool_name":"TaskUpdate","tool_input":{"taskId":"1","status":"in_progress"},
		"tool_response":{"statusChange":{"from":"pending","to":"in_progress"}}}`), spec.ChannelHooks)
	if err != nil || ev.Step == nil {
		t.Fatalf("step %v, err %v", ev.Step, err)
	}
	expectEq(t, "step", *ev.Step, models.PlanStepEvent{StepID: "1", Status: "active"})

	created, err := inbound.Parse(d, spec.HookPlanStep, []byte(`{"session_id":"s","transcript_path":"/t",
		"hook_event_name":"TaskCreated","task_id":"2","task_subject":"Write it","task_description":"all of it"}`), spec.ChannelHooks)
	if err != nil || created.Step == nil {
		t.Fatalf("step %v, err %v", created.Step, err)
	}
	expectEq(t, "created", *created.Step, models.PlanStepEvent{StepID: "2", Text: "Write it", Detail: "all of it", Status: "pending"})

	renamed, err := inbound.Parse(d, spec.HookPlanStep, []byte(`{"session_id":"s","transcript_path":"/t",
		"hook_event_name":"PostToolUse","tool_name":"TaskUpdate","tool_input":{"taskId":"1","subject":"renamed"}}`), spec.ChannelHooks)
	if err != nil || renamed.Step == nil {
		t.Fatalf("step %v, err %v", renamed.Step, err)
	}
	expectEq(t, "a rename says no status", *renamed.Step, models.PlanStepEvent{StepID: "1", Text: "renamed"})
}

// UNVERIFIED shapes: no claude login was available to record a background call, so these
// follow the documented tool_response fields (backgroundTaskId, taskId, task_id).
const backgroundBash = `{"session_id":"s","hook_event_name":"PostToolUse","tool_name":"Bash",
	"tool_input":{"command":"npm run dev","description":"Dev server","run_in_background":true},
	"tool_response":{"stdout":"","stderr":"","interrupted":false,"backgroundTaskId":"bg1a2b3c4"},
	"tool_use_id":"toolu_bg"}`

func TestClaudeDescriptor_ABackgroundedCommandNamesTheShellItStarted(t *testing.T) {
	d := loadDescriptor(t, "claude")

	bash := parseRaw(t, d, "tool_post", []byte(backgroundBash), spec.ChannelHooks)
	expectEq(t, "bash shell", bash.SpawnsShell, "bg1a2b3c4")
	expectEq(t, "bash kind", bash.Kind, "execute")
	expectEq(t, "bash command", bash.Target, "npm run dev")

	monitor := parseRaw(t, d, "tool_post", []byte(`{"session_id":"s","hook_event_name":"PostToolUse","tool_name":"Monitor",
		"tool_input":{"command":"tail -f app.log","description":"Watch the log","timeout_ms":300000},
		"tool_response":{"taskId":"mon9z8y7","timeoutMs":300000},"tool_use_id":"toolu_mon"}`), spec.ChannelHooks)
	expectEq(t, "monitor shell", monitor.SpawnsShell, "mon9z8y7")
	expectEq(t, "monitor kind", monitor.Kind, "execute")
	expectEq(t, "monitor is a monitor", monitor.ShellKind, "monitor")
	expectEq(t, "monitor timeout", monitor.ShellTimeoutMS, 300000)
	expectEq(t, "bash is a shell", bash.ShellKind, "shell")

	persistent := parseRaw(t, d, "tool_post", []byte(`{"session_id":"s","hook_event_name":"PostToolUse","tool_name":"Monitor",
		"cwd":"/work","tool_input":{"command":"tail -f app.log","persistent":true},
		"tool_response":{"taskId":"mon1"},"tool_use_id":"toolu_mon2"}`), spec.ChannelHooks)
	expectEq(t, "persistent", persistent.ShellPersistent, true)
	expectEq(t, "cwd", persistent.Cwd, "/work")

	plain := parseRecorded(t, d, "tool_post", "PostToolUse.Bash.json", spec.ChannelHooks)
	expectEq(t, "a foreground command starts none", plain.SpawnsShell, "")
}

func TestClaudeDescriptor_AStopCallNamesTheShellItStopped(t *testing.T) {
	d := loadDescriptor(t, "claude")

	stop := parseRaw(t, d, "tool_post", []byte(`{"session_id":"s","hook_event_name":"PostToolUse","tool_name":"TaskStop",
		"tool_input":{"task_id":"bg1a2b3c4"},"tool_response":{"message":"Stopped","task_id":"bg1a2b3c4","task_type":"shell"},
		"tool_use_id":"toolu_stop"}`), spec.ChannelHooks)
	expectEq(t, "stopped", stop.StopsShell, "bg1a2b3c4")
	expectEq(t, "spawns", stop.SpawnsShell, "")
}

func TestClaudeDescriptor_ATaskNotificationNamesTheShellItEnds(t *testing.T) {
	d := loadDescriptor(t, "claude")
	for status, want := range map[string]string{"completed": "finished", "failed": "failed", "stopped": "stopped"} {
		prompt := "<task-notification>\n<task-id>bg1a2b3c4</task-id>\n<tool-use-id>toolu_bg</tool-use-id>\n" +
			"<output-file>/tmp/out</output-file>\n<status>" + status + "</status>\n<summary>Background command finished</summary>\n</task-notification>"
		raw, err := json.Marshal(map[string]any{"session_id": "s", "hook_event_name": "UserPromptSubmit", "prompt": prompt})
		if err != nil {
			t.Fatal(err)
		}
		ev, err := inbound.Parse(d, spec.HookUserPrompt, raw, spec.ChannelHooks)
		if err != nil || ev.Shell == nil {
			t.Fatalf("shell %v, err %v", ev.Shell, err)
		}
		expectEq(t, "kind", ev.Kind, spec.HookUserPrompt)
		expectEq(t, "shell", *ev.Shell, models.ShellEvent{ID: "bg1a2b3c4", Status: want, Summary: "Background command finished"})
	}

	event := "<task-notification>\n<task-id>mon9z8y7</task-id>\n<summary>Monitor event</summary>\n<event>ERROR: db down\nretrying</event>\n</task-notification>"
	raw, err := json.Marshal(map[string]any{"session_id": "s", "hook_event_name": "UserPromptSubmit", "prompt": event})
	if err != nil {
		t.Fatal(err)
	}
	line, err := inbound.Parse(d, spec.HookUserPrompt, raw, spec.ChannelHooks)
	if err != nil || line.Shell == nil {
		t.Fatalf("shell %v, err %v", line.Shell, err)
	}
	expectEq(t, "a monitor's line ends nothing", *line.Shell, models.ShellEvent{
		ID: "mon9z8y7", Summary: "Monitor event", Note: "ERROR: db down\nretrying",
	})

	typed, err := inbound.Parse(d, spec.HookUserPrompt, []byte(`{"session_id":"s","prompt":"hello"}`), spec.ChannelHooks)
	if err != nil {
		t.Fatal(err)
	}
	expectEq(t, "a typed prompt ends none", typed.Shell == nil, true)
}
