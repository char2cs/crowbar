package transcript

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// Choice is one ask as the choice a client shows, and whether Crowbar holds
// the reply an answer would reach.
type Choice struct {
	domain.ActivityChoice
	Answerable bool
}

// InterruptionID is the id the interruption an ask raises is listed under.
func InterruptionID(askID string) string {
	return "interrupt-" + strings.TrimPrefix(askID, "choice-")
}

type askFact struct {
	ask        ledger.Ask
	turnID     string
	seq        int64
	at         time.Time
	resolved   *ledger.AskResolved
	resolvedAt time.Time
}

func (f *facts) applyAsk(r ledger.Row) {
	body, err := ledger.DecodeBody[ledger.Ask](r)
	if err != nil || body.AskID == "" || f.asks[body.AskID] != nil {
		return
	}
	a := &askFact{ask: body, turnID: r.TurnID, seq: r.Seq, at: r.At}
	f.asks[body.AskID] = a
	f.asked = append(f.asked, a)
}

func (f *facts) applyAskResolved(r ledger.Row) {
	body, err := ledger.DecodeBody[ledger.AskResolved](r)
	if err != nil {
		return
	}
	if a := f.asks[body.AskID]; a != nil && a.resolved == nil {
		a.resolved, a.resolvedAt = &body, r.At
	}
}

// askResolution is when and how a's ask closed: by its own row, else
// abandoned when its turn or its runner ended after it was raised.
func (f *facts) askResolution(a *askFact) (*time.Time, string, []string) {
	if a.resolved != nil {
		at := a.resolvedAt
		return &at, a.resolved.Resolution, a.resolved.OptionIDs
	}
	if at, over := f.closedBy(a.turnID, a.seq, true); over {
		return &at, ledger.ResolutionAbandoned, nil
	}
	return nil, "", nil
}

// choices are the chat's asks that offered a choice, one each, by their row.
func (f *facts) choices() []Choice {
	out := make([]Choice, 0, len(f.asked))
	for _, a := range f.asked {
		if a.ask.Kind == "" {
			continue
		}
		c := choiceOf(f.chatID, a)
		var optionIDs []string
		c.ResolvedAt, c.Resolution, optionIDs = f.askResolution(a)
		if c.Resolution == ledger.ResolutionAnswered {
			c.AnsweredOptionIDs = optionIDs
		}
		out = append(out, Choice{ActivityChoice: c, Answerable: a.ask.Answerable && c.ResolvedAt == nil})
	}
	return out
}

// PendingChoicesOf are the choices st's open asks offer, read from the state
// alone: what a publication under the chat's writer announces.
func PendingChoicesOf(st ledger.State) []domain.ActivityChoice {
	var out []domain.ActivityChoice
	for _, p := range st.PendingAsks() {
		if p.Kind != "" {
			out = append(out, choiceOf(st.ChatID, &askFact{ask: p.Ask, turnID: p.TurnID, seq: p.Seq, at: p.At}))
		}
	}
	return out
}

func choiceOf(chatID string, a *askFact) domain.ActivityChoice {
	c := domain.ActivityChoice{
		ID: a.ask.AskID, TurnID: a.turnID, ChatID: chatID, Seq: a.seq, Kind: a.ask.Kind,
		PromptID: a.ask.PromptID, ToolID: a.ask.ToolID, ToolName: a.ask.ToolName, Title: a.ask.Title,
		Question: a.ask.Question, Mode: a.ask.Mode, Multi: a.ask.Multi, At: a.at,
		Options:   decoded[[]domain.ActivityChoiceOption](a.ask.Options),
		Questions: decoded[[]domain.ActivityChoiceQuestion](a.ask.Questions),
	}
	if len(a.ask.Schema) > 0 {
		c.Schema = string(a.ask.Schema)
	}
	return c
}

// decoded is raw as T. The mapper wrote it from these same shapes; one that
// does not decode shows the question without it.
func decoded[T any](raw json.RawMessage) T {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		var none T
		return none
	}
	return v
}
