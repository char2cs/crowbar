package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Activity serves what the agent DID during a chat — tool calls, subagents and
// interruptions — as distinct from what it said.
//
// It is a separate resource from the messages, not a field on them: the
// conversation is read on every render, while activity is read when a user opens
// a timeline, and one is orders of magnitude larger than the other.
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

	activity, err := h.usecase.ReadActivity(ctx.Request.Context(), chat.ID, int64(after), limit)
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
			// The refs themselves never cross this boundary: they are internal
			// addresses, and the client asks for a payload by tool id instead.
			HasRequest: c.RequestRef != "", HasResult: c.ResultRef != "",
			StartedAt: c.StartedAt, EndedAt: c.EndedAt,
		})
	}
	for _, s := range activity.Subagents {
		out.Subagents = append(out.Subagents, dto.AgentSubagentDTO{
			ID: s.ID, TurnID: s.TurnID, Seq: s.Seq, AgentType: s.AgentType,
			StartedAt: s.StartedAt, EndedAt: s.EndedAt,
		})
	}
	for _, i := range activity.Interruptions {
		out.Interruptions = append(out.Interruptions, dto.AgentInterruptionDTO{
			ID: i.ID, TurnID: i.TurnID, Seq: i.Seq, Kind: i.Kind, Detail: i.Detail,
			At: i.At, ResolvedAt: i.ResolvedAt,
		})
	}
	libs.WriteQueryOK(ctx, out)
}

// Choices serves the prompts a chat is still waiting on a human to answer.
//
// It is a resource of its own rather than a field on the activity timeline
// because it answers a different question at a different rate: the timeline is
// read when a user opens it, while "is this agent blocked on me" is asked
// constantly and must not drag five hundred tool calls behind it.
//
// A chat waiting on nothing yields an empty list, not a 404: "no prompt" is an
// answer, and a client that renders nothing for it is correct.
func (h *Handlers) Choices(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	choices, err := h.usecase.ReadPendingChoices(ctx.Request.Context(), chat.ID)
	if err != nil {
		status, message := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, message)
		return
	}
	libs.WriteQueryOK(ctx, h.choiceDTOs(chat.ID, choices))
}

// choiceDTOs renders prompts, stamping each with whether it can still be answered
// from here.
//
// Answerability is asked ONCE for the whole set rather than per row: it is a
// lookup against the desk of relays currently blocked, and a per-row call would
// take that lock as many times as a turn had questions.
func (h *Handlers) choiceDTOs(chatID string, in []domain.ActivityChoice) []dto.AgentChoiceDTO {
	answerable := map[string]bool{}
	for _, id := range h.usecase.AnswerableChoiceIDs(chatID, in) {
		answerable[id] = true
	}
	out := make([]dto.AgentChoiceDTO, 0, len(in))
	for _, c := range in {
		out = append(out, dto.AgentChoiceDTO{
			ID: c.ID, TurnID: c.TurnID, Seq: c.Seq, Kind: c.Kind,
			ToolName: c.ToolName, Title: c.Title, Question: c.Question,
			Mode: c.Mode, Multi: c.Multi, Options: choiceOptionDTOs(c.Options),
			Schema:     c.Schema,
			Pending:    c.Pending(),
			Answerable: answerable[c.ID],
			At:         c.At, ResolvedAt: c.ResolvedAt, Resolution: c.Resolution,
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

// ToolPayload serves one tool call's request or result.
//
// It is fetched on demand rather than shipped with the timeline because a coding
// agent produces hundreds of KB per turn, and almost none of it is ever looked at.
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

	payload, err := h.usecase.ReadToolPayload(
		ctx.Request.Context(), chat.ID, ctx.Param("toolId"), side)
	if errors.Is(err, agentactivity.ErrNotFound) {
		// Retention may legitimately have swept it. That is a fact about the
		// payload, not a failure of the request.
		libs.WriteErr(ctx, http.StatusNotFound, "payload is no longer available")
		return
	}
	if err != nil {
		status, message := libs.StatusAndMessage(err)
		libs.WriteErr(ctx, status, message)
		return
	}
	// Served as plain bytes: a tool payload is whatever the provider produced, and
	// re-encoding it as JSON would corrupt the very thing the user asked to see.
	ctx.Data(http.StatusOK, "text/plain; charset=utf-8", payload)
}

// Telemetry serves the provider's own report of context, cost, rate limits and
// resolved model.
//
// A chat with no report yields 204: the provider has not said anything, which is
// different from having said zero, and the client draws no gauge rather than an
// empty one.
func (h *Handlers) Telemetry(ctx *gin.Context) {
	chat, ok := h.requireChatInWorkspace(ctx, ctx.Param("id"))
	if !ok {
		return
	}
	report, ok := h.usecase.Telemetry(chat.ID)
	if !ok {
		ctx.Status(http.StatusNoContent)
		return
	}

	out := dto.AgentTelemetryDTO{ObservedAt: report.ObservedAt, Source: report.Source}
	if c := report.Context; c != nil {
		out.Context = &dto.AgentContextUsageDTO{
			CapacityTokens: c.CapacityTokens, UsedTokens: c.UsedTokens,
			UsedPercent: c.UsedPercent, RemainingPercent: c.RemainingPercent,
		}
	}
	for _, w := range report.RateLimits {
		out.RateLimits = append(out.RateLimits, dto.AgentRateLimitDTO{
			ID: w.ID, Label: w.Label, UsedPercent: w.UsedPercent, ResetsAt: w.ResetsAt,
		})
	}
	if c := report.Cost; c != nil {
		out.Cost = &dto.AgentSessionCostDTO{TotalUSD: c.TotalUSD, APIDurationMS: c.APIDurationMS}
	}
	if m := report.Model; m != nil {
		out.Model = &dto.AgentModelIdentityDTO{ID: m.ID, DisplayName: m.DisplayName}
	}
	libs.WriteQueryOK(ctx, out)
}
