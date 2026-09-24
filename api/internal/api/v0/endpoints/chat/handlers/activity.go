package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func (h *Handlers) Activity(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	after, ok := intQuery(ctx, "after")
	if !ok {
		return
	}
	limit, ok := intQuery(ctx, "limit")
	if !ok {
		return
	}

	activity, err := h.turns.ReadActivity(ctx.Request.Context(), chat.ID, int64(after), limit)
	if err != nil {
		status, message := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, message)
		return
	}

	out := dto.AgentActivityDTO{
		ToolCalls:     make([]dto.AgentToolCallDTO, 0, len(activity.ToolCalls)),
		Subagents:     make([]dto.AgentSubagentDTO, 0, len(activity.Subagents)),
		Interruptions: make([]dto.AgentInterruptionDTO, 0, len(activity.Interruptions)),
		Choices:       h.choiceDTOs(chat.ID, activity.Choices),
	}
	for _, c := range activity.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, dto.AgentToolCallDTO{
			ID: c.ID, TurnID: c.TurnID, Seq: c.Seq, Name: c.Name, Target: c.Target,
			Status: c.Status, Error: c.Error, DurationMS: c.DurationMS,

			HasRequest: c.RequestRef != "", HasResult: c.ResultRef != "",
			SubagentID: c.SubagentID,
			StartedAt:  c.StartedAt, EndedAt: c.EndedAt,
		})
	}
	for _, s := range activity.Subagents {
		out.Subagents = append(out.Subagents, dto.AgentSubagentDTO{
			ID: s.ID, TurnID: s.TurnID, Seq: s.Seq, AgentType: s.AgentType,
			StartedAt: s.StartedAt, EndedAt: s.EndedAt,
			Messages: subagentMessageDTOs(s.Messages),
		})
	}
	for _, i := range activity.Interruptions {
		out.Interruptions = append(out.Interruptions, dto.AgentInterruptionDTO{
			ID: i.ID, TurnID: i.TurnID, Seq: i.Seq, DisplayOrder: i.DisplayOrder,
			Kind: i.Kind, Detail: i.Detail,
			At: i.At, ResolvedAt: i.ResolvedAt,
		})
	}
	libs.WriteQueryOK(ctx, out)
}

func subagentMessageDTOs(in []domain.ActivitySubagentMessage) []dto.AgentSubagentMessageDTO {
	if len(in) == 0 {
		return nil
	}
	out := make([]dto.AgentSubagentMessageDTO, 0, len(in))
	for _, m := range in {
		out = append(out, dto.AgentSubagentMessageDTO{Text: m.Text, At: m.At})
	}
	return out
}

func (h *Handlers) Choices(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	choices, err := h.turns.ReadPendingChoices(ctx.Request.Context(), chat.ID)
	if err != nil {
		status, message := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, message)
		return
	}
	libs.WriteQueryOK(ctx, h.choiceDTOs(chat.ID, choices))
}

func (h *Handlers) choiceDTOs(chatID string, in []domain.ActivityChoice) []dto.AgentChoiceDTO {
	answerable := map[string]bool{}
	for _, id := range h.answers.AnswerableChoiceIDs(chatID, in) {
		answerable[id] = true
	}
	out := make([]dto.AgentChoiceDTO, 0, len(in))
	for _, c := range in {
		out = append(out, dto.AgentChoiceDTO{
			ID: c.ID, TurnID: c.TurnID, Seq: c.Seq, Kind: c.Kind,
			ToolName: c.ToolName, Title: c.Title, Question: c.Question,
			Mode: c.Mode, Multi: c.Multi, Options: choiceOptionDTOs(c.Options),
			Questions:  choiceQuestionDTOs(c.Questions),
			Schema:     c.Schema,
			Pending:    c.Pending(),
			Answerable: answerable[c.ID],
			At:         c.At, ResolvedAt: c.ResolvedAt, Resolution: c.Resolution,
			AutoApproved:      c.AutoApproved,
			AnsweredOptionIDs: c.AnsweredOptionIDs,
		})
	}
	return out
}

func choiceQuestionDTOs(in []domain.ActivityChoiceQuestion) []dto.AgentChoiceQuestionDTO {
	if len(in) == 0 {
		return nil
	}
	out := make([]dto.AgentChoiceQuestionDTO, 0, len(in))
	for _, q := range in {
		out = append(out, dto.AgentChoiceQuestionDTO{
			ID: q.ID, Title: q.Title, Text: q.Text, Multi: q.Multi,
			Options: choiceOptionDTOs(q.Options),
		})
	}
	return out
}

func choiceOptionDTOs(in []domain.ActivityChoiceOption) []dto.AgentChoiceOptionDTO {
	out := make([]dto.AgentChoiceOptionDTO, 0, len(in))
	for _, o := range in {
		out = append(out, dto.AgentChoiceOptionDTO{
			ID: o.ID, Kind: o.Kind, Label: o.Label, Description: o.Description,
		})
	}
	return out
}

func (h *Handlers) ToolPayload(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	side := ctx.Query("side")
	if side != "request" && side != "result" {
		libs.WriteErr(ctx, http.StatusBadRequest, "side must be request or result")
		return
	}

	payload, err := h.turns.ReadToolPayload(
		ctx.Request.Context(), chat.ID, ctx.Param("toolId"), side,
	)
	if errors.Is(err, agentactivity.ErrNotFound) {
		libs.WriteErr(ctx, http.StatusNotFound, "payload is no longer available")
		return
	}
	if err != nil {
		status, message := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, message)
		return
	}

	ctx.Data(http.StatusOK, "text/plain; charset=utf-8", payload)
}

func (h *Handlers) Telemetry(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	report, ok := h.turns.Telemetry(chat.ID)
	// Absence, never a dead control: a chat on a surface whose channel
	// carries no usage report has a number that can never move again, and
	// serving the last one it earned elsewhere is worse than no gauge.
	if !ok || !h.turns.TelemetryOnChatSurface(ctx.Request.Context(), chat.ID) {
		ctx.Status(http.StatusNoContent)
		return
	}

	out := dto.AgentTelemetryDTOFrom(report)
	libs.WriteQueryOK(ctx, out)
}
