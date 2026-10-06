package dto

import "github.com/char2cs/crowbar/api/internal/domain"

// AgentChatKindChoice announces that the set of prompts a chat is BLOCKED on has
// changed: one opened, was answered, resolved at the terminal, expired or stopped
// being answerable.
//
// The whole current set rides the frame in Choices, so the newest one is the
// entire truth and a client that missed one is correct again on the next. An
// absent or empty Choices clears the chat's prompts. A socket's first frames
// carry this kind for every chat already waiting (the stream's snapshot), which
// is why a client needs no read of its own to learn what is pending.
//
// Every prompt the person must answer rides it, whoever raised it: nothing here
// filters on a subagent, because a prompt a subagent blocks on is still one only
// the person can unblock.
const AgentChatKindChoice = "choice"

// AgentChoiceDTOsFrom is the wire form of a chat's prompts. answerable names the
// ones a relay is holding open right now.
func AgentChoiceDTOsFrom(in []domain.ActivityChoice, answerable []string) []AgentChoiceDTO {
	held := make(map[string]bool, len(answerable))
	for _, id := range answerable {
		held[id] = true
	}
	out := make([]AgentChoiceDTO, 0, len(in))
	for _, c := range in {
		out = append(out, AgentChoiceDTO{
			ID: c.ID, TurnID: c.TurnID, Seq: c.Seq, Kind: c.Kind,
			ToolName: c.ToolName, Title: c.Title, Question: c.Question,
			Mode: c.Mode, Multi: c.Multi, Options: choiceOptionDTOs(c.Options),
			Questions:  choiceQuestionDTOs(c.Questions),
			Schema:     c.Schema,
			Pending:    c.Pending(),
			Answerable: held[c.ID],
			At:         c.At, ResolvedAt: c.ResolvedAt, Resolution: c.Resolution,
			AutoApproved:      c.AutoApproved,
			AnsweredOptionIDs: c.AnsweredOptionIDs,
		})
	}
	return out
}

func choiceQuestionDTOs(in []domain.ActivityChoiceQuestion) []AgentChoiceQuestionDTO {
	if len(in) == 0 {
		return nil
	}
	out := make([]AgentChoiceQuestionDTO, 0, len(in))
	for _, q := range in {
		out = append(out, AgentChoiceQuestionDTO{
			ID: q.ID, Title: q.Title, Text: q.Text, Multi: q.Multi,
			Options: choiceOptionDTOs(q.Options),
		})
	}
	return out
}

func choiceOptionDTOs(in []domain.ActivityChoiceOption) []AgentChoiceOptionDTO {
	out := make([]AgentChoiceOptionDTO, 0, len(in))
	for _, o := range in {
		out = append(out, AgentChoiceOptionDTO{
			ID: o.ID, Kind: o.Kind, Label: o.Label, Description: o.Description,
		})
	}
	return out
}
