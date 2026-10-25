package ingress

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// maxAskInput is how much of its call's input an ask row keeps.
const maxAskInput = 16 << 10

// mapAsk records a question the CLI is blocked on, bound to the open tool of
// its scope that raised it. A retried delivery joins the ask it repeats.
func mapAsk(st ledger.State, ev engineagents.CanonicalEvent, src Source, scope string) ([]ledger.Row, Effects, error) {
	a, err := askOf(ev)
	if err != nil {
		return nil, Effects{}, err
	}
	if c := ev.Choice; c != nil && c.ToolName != "" {
		a.ToolID, _ = st.OpenTool(scope, c.ToolName, ledger.InputKey(c.ToolInput))
	}
	if id, ok := st.SameAsk(scope, a); ok {
		return nil, Effects{HoldAsk: heldAgain(st, id, src)}, nil
	}
	live := startsOn(st, src.RunnerID)
	held := src.Answerable && a.Kind != "" && live
	a.AskID = "choice-" + uuid.NewString()
	a.Answerable = held
	turnID, _ := turnFor(st, src, scope, ev.TurnID)
	row, err := ledger.NewRow(ledger.KindAsk, turnID, scope, a)
	if err != nil {
		return nil, Effects{}, err
	}
	if held {
		return []ledger.Row{row}, Effects{HoldAsk: a.AskID}, nil
	}
	if live && waitedOn(st, scope) {
		return []ledger.Row{row}, Effects{}, nil
	}
	// Unheld, a past runner's or one no end closes: nothing waits on it.
	closed, err := ledger.NewRow(ledger.KindAskResolved, turnID, scope,
		ledger.AskResolved{AskID: a.AskID, Resolution: ledger.ResolutionAbandoned})
	if err != nil {
		return nil, Effects{}, err
	}
	return []ledger.Row{row, closed}, Effects{}, nil
}

// waitedOn reports whether an ask in scope is waited on: the chat's agent's
// while a turn runs, a nested conversation's while no turn end outlived it.
func waitedOn(st ledger.State, scope string) bool {
	if scope == "" || !st.HasScopePhase(scope, ledger.SubagentStart) {
		_, running := st.RunningTurn()
		return running
	}
	return st.ScopeWaits(scope)
}

// heldAgain is the ask a repeated delivery's reply is held on: the one it
// repeats, while Crowbar can still hold it.
func heldAgain(st ledger.State, askID string, src Source) string {
	for _, p := range st.PendingAsks() {
		if p.AskID == askID && p.Answerable && src.Answerable {
			return askID
		}
	}
	return ""
}

func askOf(ev engineagents.CanonicalEvent) (ledger.Ask, error) {
	a := ledger.Ask{Event: ev.Kind}
	if ev.Interrupt != nil {
		a.Detail = ev.Interrupt.Detail
	}
	c := ev.Choice
	if c == nil {
		return a, nil
	}
	a.Kind, a.PromptID, a.ToolName = c.Kind, c.PromptID, c.ToolName
	a.Title, a.Question, a.Mode, a.Multi = c.Title, c.Question, c.Mode, c.Multi
	a.ToolTarget, a.MCPServer = c.ToolTarget, c.MCPServer
	var err error
	if a.ToolInput, a.ToolInputTruncated, err = askInput(c.ToolInput); err != nil {
		return a, fmt.Errorf("ingress: ask input: %w", err)
	}
	if len(c.Options) > 0 {
		if a.Options, err = json.Marshal(choiceOptions(c.Options)); err != nil {
			return a, fmt.Errorf("ingress: ask options: %w", err)
		}
	}
	if len(c.Questions) > 0 {
		if a.Questions, err = json.Marshal(choiceQuestions(c.Questions)); err != nil {
			return a, fmt.Errorf("ingress: ask questions: %w", err)
		}
	}
	if json.Valid(c.Schema) {
		a.Schema = json.RawMessage(c.Schema)
	}
	return a, nil
}

// askInput is input as an ask row keeps it: whole while it is JSON of at most
// maxAskInput bytes, else as its leading text, cut there if it is longer.
func askInput(input []byte) (json.RawMessage, bool, error) {
	if len(input) == 0 {
		return nil, false, nil
	}
	if len(input) <= maxAskInput && json.Valid(input) {
		return json.RawMessage(input), false, nil
	}
	lead, err := json.Marshal(cut(string(input), maxAskInput))
	return lead, len(input) > maxAskInput, err
}

func choiceQuestions(in []engineagents.PromptQuestion) []domain.ActivityChoiceQuestion {
	out := make([]domain.ActivityChoiceQuestion, 0, len(in))
	for _, q := range in {
		out = append(out, domain.ActivityChoiceQuestion{
			ID: q.ID, Title: q.Title, Text: q.Text, Multi: q.Multi, Options: choiceOptions(q.Options),
		})
	}
	return out
}

func choiceOptions(in []engineagents.ChoiceOption) []domain.ActivityChoiceOption {
	if len(in) == 0 {
		return nil
	}
	out := make([]domain.ActivityChoiceOption, 0, len(in))
	for _, o := range in {
		out = append(out, domain.ActivityChoiceOption{ID: o.ID, Kind: o.Kind, Label: o.Label, Description: o.Description})
	}
	return out
}
