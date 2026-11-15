package descriptor_test

import (
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
