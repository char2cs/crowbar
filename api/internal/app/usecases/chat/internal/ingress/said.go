package ingress

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// endsText reports whether an event of kind is the agent acting: what it
// streamed before is whole when its descriptor says messages end at an action.
func endsText(kind string) bool {
	switch kind {
	case engineagents.HookToolPre, engineagents.HookSubagentPre, engineagents.HookPermission, engineagents.HookElicitation:
		return true
	}
	return false
}

// textBefore records the answers src streamed with no row of their own before
// ev ended them, so they read above what ev records.
func textBefore(st ledger.State, ev engineagents.CanonicalEvent, src Source) ([]ledger.Row, error) {
	turnID, ok := turnFor(st, src, "", ev.TurnID)
	if !ok {
		return nil, nil
	}
	return answerRows(turnID, unsaid(st, turnID, src))
}

// answerRows are turnID's main-scope messages as whole rows.
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

// closingRows are what a turn's end records: answers streamed with no row of their
// own, the hook's reply and its thought, and every ask nothing will answer now. One
// message is one whole row; the reply is its last message's (withReply).
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

// withReply is said with the turn's reply in it, and the turn's last message. The
// reply joins the last message still open; after a whole one it is said only as
// far as that message did not already say it.
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

// afterWhole is the reply of a turn whose last message is whole: what that
// message does not say is the turn's reply, recorded once.
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

// notYetSaid is the part of reply a whole message's text does not hold, edge
// spaces aside: nothing when the message says it, the rest when it extends it.
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

// fuller is the longer of a turn end's reply and the stream it names: the
// reply may be another message's text, the stream may have lost frames.
func fuller(reply, streamed string) string {
	if len(reply) > len(streamed) {
		return reply
	}
	return streamed
}

// unsaid are turnID's main-scope answers that streamed with no whole row. A
// whole row cut at an action is said again with what streamed after it, which
// is all a stream holds once its row is in.
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

// thoughtRows are the thoughts of a turn as rows: one per message of it, said or
// recorded, that a thought names, and one for the last message holding the newest of the rest.
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
