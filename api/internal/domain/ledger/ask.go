package ledger

import (
	"cmp"
	"encoding/json"
	"slices"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

const (
	ResolutionAnswered  = "answered"
	ResolutionProceeded = "proceeded"
	ResolutionRefused   = "refused"
	ResolutionAbandoned = "abandoned"
)

type Ask struct {
	AskID              string          `json:"askId"`
	Event              string          `json:"event"`
	Kind               string          `json:"kind,omitempty"`
	PromptID           string          `json:"promptId,omitempty"`
	ToolID             string          `json:"toolId,omitempty"`
	ToolName           string          `json:"toolName,omitempty"`
	Title              string          `json:"title,omitempty"`
	Question           string          `json:"question,omitempty"`
	Mode               string          `json:"mode,omitempty"`
	Multi              bool            `json:"multi,omitempty"`
	Options            json.RawMessage `json:"options,omitempty"`
	Questions          json.RawMessage `json:"questions,omitempty"`
	Schema             json.RawMessage `json:"schema,omitempty"`
	Detail             string          `json:"detail,omitempty"`
	ToolTarget         string          `json:"toolTarget,omitempty"`
	ToolInput          json.RawMessage `json:"toolInput,omitempty"`
	ToolInputTruncated bool            `json:"toolInputTruncated,omitempty"`
	MCPServer          string          `json:"mcpServer,omitempty"`
	InputKey           string          `json:"inputKey,omitempty"`
	Answerable         bool            `json:"answerable"`
}

func (a Ask) OutlivesEnd(subagentRunning bool) bool { return subagentRunning && a.Answerable }

type AskResolved struct {
	AskID      string   `json:"askId"`
	Resolution string   `json:"resolution"`
	OptionIDs  []string `json:"optionIds,omitempty"`
}

type PendingAsk struct {
	Ask
	TurnID, Scope string
	Seq           int64
	At            time.Time
}

func (s State) PendingAsks() []PendingAsk {
	out := make([]PendingAsk, 0, len(s.keys.asks))
	for _, a := range s.keys.asks {
		out = append(out, a)
	}
	slices.SortFunc(out, func(a, b PendingAsk) int { return cmp.Compare(a.Seq, b.Seq) })
	return out
}

func (s State) OpenAsks() []domain.ActivityChoice {
	var out []domain.ActivityChoice
	for _, p := range s.PendingAsks() {
		if p.Kind == "" {
			continue
		}
		c := domain.ActivityChoice{
			ID: p.AskID, TurnID: p.TurnID, ChatID: s.ChatID, Seq: p.Seq, Kind: p.Kind,
			PromptID: p.PromptID, ToolID: p.ToolID, ToolName: p.ToolName, Title: p.Title,
			ToolTarget: p.ToolTarget, MCPServer: p.MCPServer,
			Question: p.Question, Mode: p.Mode, Multi: p.Multi, At: p.At,
			Options:   decoded[[]domain.ActivityChoiceOption](p.Options),
			Questions: decoded[[]domain.ActivityChoiceQuestion](p.Questions),
		}
		if len(p.Schema) > 0 {
			c.Schema = string(p.Schema)
		}
		out = append(out, c)
	}
	return out
}

func decoded[T any](raw json.RawMessage) T {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		var none T
		return none
	}
	return v
}

func (s State) SameAsk(scope string, a Ask) (string, bool) {
	for _, p := range s.PendingAsks() {
		if p.Scope != scope || p.Event != a.Event || otherInput(a, p.Ask) {
			continue
		}
		if a.ToolID != "" || p.ToolID != "" {
			if p.ToolID == a.ToolID {
				return p.AskID, true
			}
			continue
		}
		if p.PromptID == a.PromptID && p.ToolName == a.ToolName && p.Question == a.Question && p.Title == a.Title {
			return p.AskID, true
		}
	}
	return "", false
}

func otherInput(a, b Ask) bool {
	return a.InputKey != "" && b.InputKey != "" && a.InputKey != b.InputKey
}

func (s State) AsksOfTool(scope, toolID string) []string {
	var ids []string
	for _, p := range s.PendingAsks() {
		if p.Scope == scope && p.ToolID == toolID {
			ids = append(ids, p.AskID)
		}
	}
	return ids
}

func ResolveAsk(st State, askID, resolution string, optionIDs []string) ([]Row, error) {
	p, ok := st.keys.asks[askID]
	if !ok {
		return nil, nil
	}
	r, err := NewRow(KindAskResolved, p.TurnID, p.Scope, AskResolved{
		AskID: askID, Resolution: resolution, OptionIDs: optionIDs,
	})
	return []Row{r}, err
}

func (s *State) applyAsk(r Row) {
	body, err := DecodeBody[Ask](r)
	if err != nil {
		return
	}
	s.keys.init().asks[body.AskID] = PendingAsk{Ask: body, TurnID: r.TurnID, Scope: r.Scope, Seq: r.Seq, At: r.At}
}

func (s *State) applyAskResolved(r Row) {
	if body, err := DecodeBody[AskResolved](r); err == nil {
		delete(s.keys.asks, body.AskID)
	}
}

func (s *State) abandonAsks(which func(PendingAsk) bool) {
	for id, a := range s.keys.asks {
		if which(a) {
			delete(s.keys.asks, id)
		}
	}
}
