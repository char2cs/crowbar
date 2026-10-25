package ledger

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
)

// The phases of a tool call.
const (
	ToolPre  = "pre"  // the tool was invoked
	ToolPost = "post" // it completed
	ToolFail = "fail" // it failed or was declined
)

// ToolKindMCP is the kind of a call into an MCP server.
const ToolKindMCP = "mcp"

// ToolCall is the body of a tool_call row: one phase of one tool call.
type ToolCall struct {
	ToolID     string         `json:"toolId"`
	Phase      string         `json:"phase"`
	Name       string         `json:"name,omitempty"`
	Kind       string         `json:"kind,omitempty"`
	Title      string         `json:"title,omitempty"`
	Target     string         `json:"target,omitempty"`
	Locations  []ToolLocation `json:"locations,omitempty"`
	InputRef   string         `json:"inputRef,omitempty"`
	ResultRef  string         `json:"resultRef,omitempty"`
	Status     string         `json:"status,omitempty"`
	Error      string         `json:"error,omitempty"`
	DurationMS int            `json:"durationMs,omitempty"`
	Diff       string         `json:"diff,omitempty"`
	// MCPServer and MCPTool name a call of kind ToolKindMCP; the server is
	// absent where its channel does not say it.
	MCPServer   string `json:"mcpServer,omitempty"`
	MCPTool     string `json:"mcpTool,omitempty"`
	ExitCode    *int   `json:"exitCode,omitempty"`
	Interrupted bool   `json:"interrupted,omitempty"`
	// InputKey is InputKey of the call's input: what an ask about it names it by.
	InputKey string `json:"inputKey,omitempty"`
	// SpawnsScope is the conversation of its own this call opened or
	// addresses; its events are recorded in that scope.
	SpawnsScope string `json:"spawnsScope,omitempty"`
}

// ToolLocation is a place in a file a tool call touched.
type ToolLocation struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
}

// toolKeys is what a retried phase of one tool call is recognised by.
type toolKeys struct {
	turnID, name, input string
	phases              []string
	seq                 int64 // its first row's seq
}

// InputKey is the digest of a JSON input's canonical form, so two deliveries
// of one input match however each spelled it; "" for no input or invalid JSON.
func InputKey(input []byte) string {
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil || dec.More() {
		return ""
	}
	canonical, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// ToolTurn is the turn the tool call toolID of scope was first recorded in.
func (s State) ToolTurn(scope, toolID string) (string, bool) {
	t, ok := s.keys.tools[ref(scope, toolID)]
	return t.turnID, ok
}

// HasToolPhase reports whether phase of the tool call is already recorded.
func (s State) HasToolPhase(scope, toolID, phase string) bool {
	return slices.Contains(s.keys.tools[ref(scope, toolID)].phases, phase)
}

// OpenTool is the newest tool call of scope named name that has not completed.
// Given an input, it is the newest such call given that input, else the newest
// whose input was never recorded.
func (s State) OpenTool(scope, name, inputKey string) (string, bool) {
	var given, unknown newest
	for key, t := range s.keys.tools {
		toolScope, toolID := unref(key)
		if toolScope != scope || t.name != name || t.closed() {
			continue
		}
		switch {
		case inputKey == "" || t.input == "":
			unknown.offer(t.seq, toolID)
		case t.input == inputKey:
			given.offer(t.seq, toolID)
		}
	}
	id := cmp.Or(given.id, unknown.id)
	return id, id != ""
}

// newest keeps the id of the newest call it is offered.
type newest struct {
	seq int64
	id  string
}

func (n *newest) offer(seq int64, id string) {
	if n.id == "" || seq > n.seq {
		n.seq, n.id = seq, id
	}
}

func (t toolKeys) closed() bool {
	return slices.Contains(t.phases, ToolPost) || slices.Contains(t.phases, ToolFail)
}

func (s *State) applyToolCall(r Row) {
	body, err := DecodeBody[ToolCall](r)
	if err != nil {
		return
	}
	k := s.keys.init()
	key := ref(r.Scope, body.ToolID)
	t, seen := k.tools[key]
	if !seen {
		t = toolKeys{turnID: r.TurnID, seq: r.Seq}
	}
	t.name, t.input = cmp.Or(t.name, body.Name), cmp.Or(t.input, body.InputKey)
	if !slices.Contains(t.phases, body.Phase) {
		t.phases = append(slices.Clip(t.phases), body.Phase)
	}
	k.tools[key] = t
	if body.SpawnsScope != "" {
		s.spawnScope(body.SpawnsScope, r.TurnID)
	}
}
