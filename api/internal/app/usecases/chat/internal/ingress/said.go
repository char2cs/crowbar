package ingress

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func endsText(kind string) bool {
	switch kind {
	case engineagents.HookToolPre, engineagents.HookSubagentPre, engineagents.HookPermission, engineagents.HookElicitation:
		return true
	}
	return false
}

func textBefore(st ledger.State, ev engineagents.CanonicalEvent, src Source) ([]ledger.Row, error) {
	turnID, ok := turnFor(st, src, "", ev.TurnID)
	if !ok {
		return nil, nil
	}
	return answerRows(turnID, unsaid(st, turnID, src))
}

func answerRows(turnID string, said []liveText) ([]ledger.Row, error) {
	rows := make([]ledger.Row, 0, len(said))
	for _, a := range said {
		row, err := ledger.NewRow(ledger.KindAssistantText, turnID, "",
			ledger.AssistantText{MessageID: a.ID, Index: ledger.IndexComplete, Text: a.Text})
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func closingRows(st ledger.State, turnID, reply string, src Source, outlived bool) ([]ledger.Row, error) {
	said, last := withReply(st, turnID, reply, unsaid(st, turnID, src))
	rows, err := answerRows(turnID, said)
	if err != nil {
		return nil, err
	}
	thoughts, err := thoughtRows(st, turnID, last, said, src.Live.Reasoning)
	if err != nil {
		return nil, err
	}
	rows = append(rows, thoughts...)
	abandoned, err := abandonAsks(st, turnID, outlived)
	return append(rows, abandoned...), err
}

func withReply(st ledger.State, turnID, reply string, said []liveText) ([]liveText, string) {
	if n := len(said); n > 0 {
		said[n-1].Text = fuller(reply, said[n-1].Text)
		return said, said[n-1].ID
	}
	last, hasLast := st.LastMessage(turnID)
	if hasLast && st.HasChunk("", last, ledger.IndexComplete) {
		return afterWhole(st, turnID, reply, last)
	}
	if !hasLast {
		last = ledger.ReplyID(turnID)
	}
	if reply == "" {
		return nil, last
	}
	return []liveText{{ID: last, Text: reply}}, last
}

func afterWhole(st ledger.State, turnID, reply, last string) ([]liveText, string) {
	if _, _, replied := st.Reply(turnID); replied {
		return nil, last
	}
	rest := notYetSaid(reply, st.WholeText("", last))
	if rest == "" {
		return nil, last
	}
	id := ledger.ReplyID(turnID)
	return []liveText{{ID: id, Text: rest}}, id
}

func notYetSaid(reply, whole string) string {
	r, w := strings.TrimSpace(reply), strings.TrimSpace(whole)
	if strings.HasPrefix(w, r) {
		return ""
	}
	if w != "" && strings.HasPrefix(r, w) {
		return strings.TrimSpace(r[len(w):])
	}
	return reply
}

func fuller(reply, streamed string) string {
	if len(reply) > len(streamed) {
		return reply
	}
	return streamed
}

func unsaid(st ledger.State, turnID string, src Source) []liveText {
	var out []liveText
	for _, a := range src.Live.Answers {
		if a.TurnID != turnID || a.Scope != "" {
			continue
		}
		if st.HasChunk("", a.ID, ledger.IndexComplete) {
			if !src.EndsAtAction {
				continue
			}
			a.Text = st.WholeText("", a.ID) + a.Text
		}
		out = append(out, a)
	}
	return out
}

func thoughtRows(st ledger.State, turnID, last string, said, thoughts []liveText) ([]ledger.Row, error) {
	named := map[string]bool{}
	for _, a := range said {
		named[a.ID] = true
	}
	var order []string
	text := map[string]string{}
	for _, t := range thoughts {
		if t.TurnID != turnID {
			continue
		}
		id := last
		if of, recorded := st.MessageTurn("", t.ID); named[t.ID] || recorded && of == turnID {
			id = t.ID
		}
		if _, seen := text[id]; !seen {
			order = append(order, id)
		}
		text[id] = t.Text
	}
	rows := make([]ledger.Row, 0, len(order))
	for _, id := range order {
		row, err := ledger.NewRow(ledger.KindReasoning, turnID, "", ledger.Reasoning{MessageID: id, Text: text[id]})
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}
