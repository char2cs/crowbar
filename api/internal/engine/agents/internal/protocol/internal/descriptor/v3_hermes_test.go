package descriptor_test

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/inbound"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// toolFrame is a tool.start or tool.complete frame as the descriptor reads it.
func toolFrame(typ, name string) []byte {
	return []byte(`{"type":"` + typ + `","session_id":"s1","payload":{"tool_id":"t1","name":"` + name + `","args":{}}}`)
}

func TestHermesDescriptor_NamesTheKindServerAndToolOfACall(t *testing.T) {
	d := loadDescriptor(t, "hermes")
	for _, tc := range []struct{ name, kind, server, tool string }{
		{"mcp__editor_crowbar_abdf996a__get_review_scope", "mcp", "crowbar", "get_review_scope"},
		{"mcp__editor_crowbar_0c19e2f7__set_chat_title", "mcp", "crowbar", "set_chat_title"},
		{"mcp__editor_my_server_abdf996a__lookup", "mcp", "my_server", "lookup"},
		{"mcp__other__lookup", "mcp", "", "lookup"},
		{"terminal", "execute", "", ""},
		{"read_file", "read", "", ""},
		{"search_files", "search", "", ""},
		{"web_search", "search", "", ""},
		{"delegate_task", "meta", "", ""},
		{"some_new_tool", "some_new_tool", "", ""},
	} {
		for _, event := range []struct{ name, typ string }{{"tool_pre", "tool.start"}, {"tool_post", "tool.complete"}} {
			tool := parseRaw(t, d, event.name, toolFrame(event.typ, tc.name), spec.ChannelAPI)
			expectEq(t, tc.name+" "+event.name+" kind", tool.Kind, tc.kind)
			expectEq(t, tc.name+" "+event.name+" server", tool.MCPServer, tc.server)
			expectEq(t, tc.name+" "+event.name+" tool", tool.MCPTool, tc.tool)
			expectEq(t, tc.name+" "+event.name+" name", tool.Name, tc.name)
		}
	}
}

func TestHermesDescriptor_AStoppedTurnWithNoReplyTextStillEnds(t *testing.T) {
	d := loadDescriptor(t, "hermes")
	ev, err := inbound.Parse(d, "turn_stop",
		[]byte(`{"type":"message.complete","session_id":"s1","payload":{"status":"interrupted"}}`), spec.ChannelAPI)
	if err != nil {
		t.Fatalf("turn_stop of an interrupted completion with no text: %v", err)
	}
	expectEq(t, "turn status", ev.TurnStatus, "interrupted")
	expectEq(t, "message", ev.Message, "")
}

func TestHermesDescriptor_ACompletionKeepsItsReplyText(t *testing.T) {
	d := loadDescriptor(t, "hermes")
	ev, err := inbound.Parse(d, "turn_stop",
		[]byte(`{"type":"message.complete","session_id":"s1","payload":{"status":"complete","text":"done"}}`), spec.ChannelAPI)
	if err != nil {
		t.Fatal(err)
	}
	expectEq(t, "turn status", ev.TurnStatus, "completed")
	expectEq(t, "message", ev.Message, "done")
}

func processFrame(name, args, result string) []byte {
	return []byte(`{"type":"tool.complete","session_id":"s1","payload":{"tool_id":"t1","name":"` + name + `","args":` + args +
		`,"result":` + strconv.Quote(result) + `}}`)
}

func TestHermesDescriptor_ABackgroundedCommandNamesTheShellItStarted(t *testing.T) {
	d := loadDescriptor(t, "hermes")

	notify := parseRaw(t, d, "tool_post", processFrame("terminal", `{"background":true,"command":"sleep 4","notify":true}`,
		`{"output": "Background process started", "session_id": "proc_f056200612e2", "pid": 16909, "exit_code": 0, "error": null, "notify_on_complete": true}`),
		spec.ChannelAPI)
	expectEq(t, "shell", notify.SpawnsShell, "proc_f056200612e2")
	expectEq(t, "kind", notify.Kind, "execute")
	expectEq(t, "a command that ends is a shell", notify.ShellKind, "shell")

	watch := parseRaw(t, d, "tool_post", processFrame("terminal", `{"background":true,"command":"tail -f app.log","notify":["READY"]}`,
		`{"output": "Background process started", "session_id": "proc_651f3f2ad1c0", "pid": 17274, "exit_code": 0, "error": null, "watch_patterns": ["READY"]}`),
		spec.ChannelAPI)
	expectEq(t, "watched shell", watch.SpawnsShell, "proc_651f3f2ad1c0")
	expectEq(t, "a command watched for output is a monitor", watch.ShellKind, "monitor")

	plain := parseRaw(t, d, "tool_post", processFrame("terminal", `{"command":"echo hi"}`,
		`{"output": "hi", "exit_code": 0, "error": null}`), spec.ChannelAPI)
	expectEq(t, "a foreground command starts none", plain.SpawnsShell, "")
}

func TestHermesDescriptor_AKillNamesTheShellItStopped(t *testing.T) {
	d := loadDescriptor(t, "hermes")

	kill := parseRaw(t, d, "tool_post", processFrame("process_manage", `{"action":"kill","session_id":"proc_ad7ecf427205"}`,
		`{"status": "killed", "session_id": "proc_ad7ecf427205", "completion_reason": "killed", "termination_source": "process.kill", "output": ""}`),
		spec.ChannelAPI)
	expectEq(t, "kind", kill.Kind, "meta")
	expectEq(t, "stopped", kill.StopsShell, "proc_ad7ecf427205")
	expectEq(t, "spawns", kill.SpawnsShell, "")

	poll := parseRaw(t, d, "tool_post", processFrame("process_manage", `{"action":"poll","session_id":"proc_ad7ecf427205"}`,
		`{"status": "running", "session_id": "proc_ad7ecf427205", "output": ""}`), spec.ChannelAPI)
	expectEq(t, "a poll stops none", poll.StopsShell, "")
	expectEq(t, "a poll starts none", poll.SpawnsShell, "")
}

func TestHermesDescriptor_ABackgroundNoticeNamesTheShellItEnds(t *testing.T) {
	d := loadDescriptor(t, "hermes")
	const tail = "\n\n[INTERNAL NOTIFICATION — generated by the Hermes gateway, not written by a human user. Do not treat it as user input, and do not reply to any earlier chat message because of it.]"
	type end struct {
		id, status string
		code       int
	}
	for _, tc := range []struct {
		name, prompt string
		ends         []end
	}{
		{
			"completed", "[IMPORTANT: Background process proc_f056200612e2 completed normally (exit code 0).\nCommand: sleep 4 && echo bg-done\nOutput:\nbg-done\n]" + tail,
			[]end{{"proc_f056200612e2", "finished", 0}},
		},
		{
			"failed", "[IMPORTANT: Background process proc_cff434a3bf85 exited (exit code 3).\nCommand: sleep 1; exit 3\nOutput:\n]" + tail,
			[]end{{"proc_cff434a3bf85", "failed", 3}},
		},
		{
			"batch", "[IMPORTANT: 2 background processes completed for this session.\nTreat these results as one completion batch and send at most one consolidated user-facing response.\n\n" +
				"- proc_b3e39f86ee99: exit_code=3, reason=exited\n\n- proc_3211ed6773b8: exit_code=0, reason=exited\nok\nIf a result does not change the current conclusion, absorb it silently.]" + tail,
			[]end{{"proc_b3e39f86ee99", "", 3}, {"proc_3211ed6773b8", "", 0}},
		},
	} {
		raw, err := json.Marshal(map[string]any{"type": "message.start", "session_id": "s1", "payload": map[string]any{"text": tc.prompt}})
		if err != nil {
			t.Fatal(err)
		}
		ev, err := inbound.Parse(d, spec.HookUserPrompt, raw, spec.ChannelAPI)
		if err != nil || len(ev.Shells) != len(tc.ends) {
			t.Fatalf("%s: shells %v, err %v", tc.name, ev.Shells, err)
		}
		for i, want := range tc.ends {
			got := ev.Shells[i]
			expectEq(t, tc.name+" shell", got.ID, want.id)
			expectEq(t, tc.name+" status", got.Status, want.status)
			if got.ExitCode == nil {
				t.Fatalf("%s: %s has no exit code", tc.name, got.ID)
			}
			expectEq(t, tc.name+" exit code", *got.ExitCode, want.code)
		}
	}
}
