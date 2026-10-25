package ledger

import (
	"cmp"
	"encoding/json"
	"slices"
	"time"
)

// How an ask was closed.
const (
	ResolutionAnswered  = "answered"  // a person answered it through Crowbar
	ResolutionProceeded = "proceeded" // the CLI went on without Crowbar's answer
	ResolutionRefused   = "refused"   // nobody answered in time, so the CLI was sent a refusal
	ResolutionAbandoned = "abandoned" // nothing will answer it any more
)

// Ask is the body of an ask row: a question the CLI is blocked on.
type Ask struct {
	AskID     string          `json:"askId"`
	Event     string          `json:"event"` // the hook that raised it
	Kind      string          `json:"kind,omitempty"`
	PromptID  string          `json:"promptId,omitempty"`
	ToolID    string          `json:"toolId,omitempty"`
	ToolName  string          `json:"toolName,omitempty"`
	Title     string          `json:"title,omitempty"`
	Question  string          `json:"question,omitempty"`
	Mode      string          `json:"mode,omitempty"`
	Multi     bool            `json:"multi,omitempty"`
	Options   json.RawMessage `json:"options,omitempty"`
	Questions json.RawMessage `json:"questions,omitempty"`
	Schema    json.RawMessage `json:"schema,omitempty"`
	Detail    string          `json:"detail,omitempty"`
	// ToolTarget, ToolInput and MCPServer say what the call it asks about does.
	// An input too big to keep is its leading text, as a string, and Truncated.
	ToolTarget         string          `json:"toolTarget,omitempty"`
	ToolInput          json.RawMessage `json:"toolInput,omitempty"`
	ToolInputTruncated bool            `json:"toolInputTruncated,omitempty"`
	MCPServer          string          `json:"mcpServer,omitempty"`
	// Answerable: Crowbar holds the CLI's reply, so an answer given here reaches it.
	Answerable bool `json:"answerable"`
}

// OutlivesEnd reports whether the ask stays open past its turn's end: an end that
// declared background work (async) leaves a held ask to that work, the only
// thing that can hold the CLI on it once its turn is over.
func (a Ask) OutlivesEnd(async bool) bool { return async && a.Answerable }

// AskResolved is the body of an ask_resolved row.
type AskResolved struct {
	AskID      string   `json:"askId"`
	Resolution string   `json:"resolution"`
	OptionIDs  []string `json:"optionIds,omitempty"`
}

// PendingAsk is an ask nothing has closed.
type PendingAsk struct {
	Ask
	TurnID, Scope string
	Seq           int64
	At            time.Time
}

// PendingAsks are the chat's open asks, oldest first.
func (s State) PendingAsks() []PendingAsk {
	out := make([]PendingAsk, 0, len(s.keys.asks))
	for _, a := range s.keys.asks {
		out = append(out, a)
	}
	slices.SortFunc(out, func(a, b PendingAsk) int { return cmp.Compare(a.Seq, b.Seq) })
	return out
}

// SameAsk is the open ask a's delivery repeats: raised by the same event in
// the same scope, on the same tool call; without one, under the same prompt
// id with the same tool and question. A prompt id alone names a whole turn.
func (s State) SameAsk(scope string, a Ask) (string, bool) {
	for _, p := range s.PendingAsks() {
		if p.Scope != scope || p.Event != a.Event {
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

// AsksOfTool are the open asks bound to the tool call toolID of scope.
func (s State) AsksOfTool(scope, toolID string) []string {
	var ids []string
	for _, p := range s.PendingAsks() {
		if p.Scope == scope && p.ToolID == toolID {
			ids = append(ids, p.AskID)
		}
	}
	return ids
}

// ResolveAsk is the ask_resolved row closing askID; none once it is closed.
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

// abandonAsks drops the open asks which says: nothing will answer them.
func (s *State) abandonAsks(which func(PendingAsk) bool) {
	for id, a := range s.keys.asks {
		if which(a) {
			delete(s.keys.asks, id)
		}
	}
}
