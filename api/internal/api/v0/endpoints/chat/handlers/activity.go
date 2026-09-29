package handlers

import (
	"errors"
	"net/http"
	"time"

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
			ID: c.ID, TurnID: c.TurnID, Seq: c.Seq, Name: c.Name, Kind: c.Kind, Target: c.Target,
			Locations: toolLocationDTOs(c.Locations),
			Status:    c.Status, Error: c.Error, DurationMS: c.DurationMS,

			HasRequest: c.RequestRef != "", HasResult: c.ResultRef != "",
			Diff:       c.Diff,
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
	out.Components = activityComponentDTOs(out, activity.Turns)
	libs.WriteQueryOK(ctx, out)
}

func activityTurnComponentStatus(turn domain.ActivityTurn) string {
	switch turn.Status {
	case "active", "completed", "failed", "interrupted", "abandoned":
		return turn.Status
	case "":
		if turn.Role == domain.TurnRoleNotice {
			return "failed"
		}
		if turn.EndedAt == nil {
			return "active"
		}
		// Old records have no terminal status. Preserve their historical
		// completed interpretation without changing their stored shape.
		return "completed"
	default:
		if turn.EndedAt == nil {
			return "active"
		}
		return "interrupted"
	}
}

func lifecycleComponentUpdates(
	id string,
	createdAt time.Time,
	completedAt *time.Time,
	initialStatus string,
	terminalStatus string,
	terminalPayload map[string]any,
) []dto.AgentActivityComponentUpdateDTO {
	updates := []dto.AgentActivityComponentUpdateDTO{{
		ID: id + ":1", Seq: 1, Kind: "started", At: createdAt,
		Status: initialStatus, Payload: map[string]any{},
	}}
	if completedAt == nil {
		return updates
	}
	return append(updates, dto.AgentActivityComponentUpdateDTO{
		ID: id + ":2", Seq: 2, Kind: "status_changed", At: *completedAt,
		Status: terminalStatus, Payload: terminalPayload,
	})
}

func choiceComponentStatus(choice dto.AgentChoiceDTO) string {
	if choice.ResolvedAt == nil {
		return "pending"
	}
	if choice.Resolution == domain.ChoiceResolutionAbandoned {
		return "abandoned"
	}
	if choice.Kind == domain.ChoiceKindPermission &&
		choice.Resolution == domain.ChoiceResolutionAnswered && choiceDenied(choice) {
		return "declined"
	}
	return "completed"
}

func choiceDenied(choice dto.AgentChoiceDTO) bool {
	if len(choice.AnsweredOptionIDs) == 0 {
		return false
	}
	picked := make(map[string]struct{}, len(choice.AnsweredOptionIDs))
	for _, id := range choice.AnsweredOptionIDs {
		picked[id] = struct{}{}
	}
	options := append([]dto.AgentChoiceOptionDTO(nil), choice.Options...)
	for _, question := range choice.Questions {
		options = append(options, question.Options...)
	}
	for _, option := range options {
		if option.Kind != domain.ChoiceOptionDeny {
			continue
		}
		if _, ok := picked[option.ID]; ok {
			return true
		}
	}
	return false
}

func toolLocationDTOs(in []domain.ActivityToolLocation) []dto.AgentToolLocationDTO {
	if len(in) == 0 {
		return nil
	}
	out := make([]dto.AgentToolLocationDTO, 0, len(in))
	for _, location := range in {
		out = append(out, dto.AgentToolLocationDTO{Path: location.Path, Line: location.Line})
	}
	return out
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
