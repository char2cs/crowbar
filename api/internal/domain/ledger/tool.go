package ledger

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
)

const (
	ToolPre  = "pre"
	ToolPost = "post"
	ToolFail = "fail"
)

const (
	ToolKindMCP      = "mcp"
	ToolKindSubagent = "subagent"
	ToolKindRead     = "read"
	ToolKindEdit     = "edit"
	ToolKindExecute  = "execute"
	ToolKindMeta     = "meta"
)

type ToolCall struct {
	ToolID         string         `json:"toolId"`
	Phase          string         `json:"phase"`
	Name           string         `json:"name,omitempty"`
	Kind           string         `json:"kind,omitempty"`
	Title          string         `json:"title,omitempty"`
	Target         string         `json:"target,omitempty"`
	Locations      []ToolLocation `json:"locations,omitempty"`
	InputRef       string         `json:"inputRef,omitempty"`
	ResultRef      string         `json:"resultRef,omitempty"`
	Status         string         `json:"status,omitempty"`
	Error          string         `json:"error,omitempty"`
	DurationMS     int            `json:"durationMs,omitempty"`
	Diff           string         `json:"diff,omitempty"`
	Output         string         `json:"output,omitempty"`
	MCPServer      string         `json:"mcpServer,omitempty"`
	MCPTool        string         `json:"mcpTool,omitempty"`
	ExitCode       *int           `json:"exitCode,omitempty"`
	Interrupted    bool           `json:"interrupted,omitempty"`
	Cwd            string         `json:"cwd,omitempty"`
	Ref            string         `json:"ref,omitempty"`
	InputKey       string         `json:"inputKey,omitempty"`
	SpawnsScope    string         `json:"spawnsScope,omitempty"`
	AddressesScope string         `json:"addressesScope,omitempty"`
}

type ToolLocation struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
}

type toolKeys struct {
	turnID, name, input string
	kind, target        string
	cwd, ref            string
	phases              []string
	seq                 int64
}

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

func (s State) ToolTurn(scope, toolID string) (string, bool) {
	t, ok := s.keys.tools[ref(scope, toolID)]
	return t.turnID, ok
}

func (s State) HasToolPhase(scope, toolID, phase string) bool {
	return slices.Contains(s.keys.tools[ref(scope, toolID)].phases, phase)
}

func (s State) ToolAim(scope, toolID string) (kind, target string) {
	t := s.keys.tools[ref(scope, toolID)]
	return t.kind, t.target
}

func (s State) ToolRun(scope, toolID string) (cwd, handle string) {
	t := s.keys.tools[ref(scope, toolID)]
	return t.cwd, t.ref
}

func (s State) OpenToolOfRef(scope, processRef string) (string, bool) {
	var found newest
	for key, t := range s.keys.tools {
		toolScope, toolID := unref(key)
		if toolScope == scope && processRef != "" && t.ref == processRef && !t.closed() {
			found.offer(t.seq, toolID)
		}
	}
	return found.id, found.id != ""
}

func (s State) ToolOpen(scope, toolID string) bool {
	t, ok := s.keys.tools[ref(scope, toolID)]
	return ok && !t.closed()
}

func (s State) HasToolInput(scope, toolID string) bool {
	return s.keys.tools[ref(scope, toolID)].input != ""
}

func (s State) OpenTool(scope, name, inputKey string) (id string, bound, anyOpen bool) {
	var given, unknown newest
	for key, t := range s.keys.tools {
		toolScope, toolID := unref(key)
		if toolScope != scope || t.name != name || t.closed() {
			continue
		}
		anyOpen = true
		switch {
		case inputKey == "" || t.input == "":
			unknown.offer(t.seq, toolID)
		case t.input == inputKey:
			given.offer(t.seq, toolID)
		}
	}
	id = cmp.Or(given.id, unknown.id)
	return id, id != "", anyOpen
}

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
	t.kind, t.target = cmp.Or(t.kind, body.Kind), cmp.Or(t.target, body.Target)
	t.cwd, t.ref = cmp.Or(t.cwd, body.Cwd), cmp.Or(t.ref, body.Ref)
	if !slices.Contains(t.phases, body.Phase) {
		t.phases = append(slices.Clip(t.phases), body.Phase)
	}
	k.tools[key] = t
}
