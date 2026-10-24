package ledger

import (
	"cmp"
	"slices"
)

// The phases of a tool call.
const (
	ToolPre  = "pre"  // the tool was invoked
	ToolPost = "post" // it completed
	ToolFail = "fail" // it failed or was declined
)

// ToolCall is the body of a tool_call row: one phase of one tool call.
type ToolCall struct {
	ToolID     string         `json:"toolId"`
	Phase      string         `json:"phase"`
	Name       string         `json:"name,omitempty"`
	Kind       string         `json:"kind,omitempty"`
	Target     string         `json:"target,omitempty"`
	Locations  []ToolLocation `json:"locations,omitempty"`
	InputRef   string         `json:"inputRef,omitempty"`
	ResultRef  string         `json:"resultRef,omitempty"`
	Status     string         `json:"status,omitempty"`
	Error      string         `json:"error,omitempty"`
	DurationMS int            `json:"durationMs,omitempty"`
	Diff       string         `json:"diff,omitempty"`
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
	turnID, name string
	phases       []string
	seq          int64 // its first row's seq
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
func (s State) OpenTool(scope, name string) (string, bool) {
	best, id := int64(-1), ""
	for key, t := range s.keys.tools {
		toolScope, toolID := unref(key)
		if toolScope != scope || t.name != name || t.closed() || t.seq <= best {
			continue
		}
		best, id = t.seq, toolID
	}
	return id, id != ""
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
	t.name = cmp.Or(t.name, body.Name)
	if !slices.Contains(t.phases, body.Phase) {
		t.phases = append(slices.Clip(t.phases), body.Phase)
	}
	k.tools[key] = t
	if body.SpawnsScope != "" {
		s.spawnScope(body.SpawnsScope, r.TurnID)
	}
}
