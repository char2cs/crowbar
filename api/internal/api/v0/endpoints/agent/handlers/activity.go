package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
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
	}
	for _, c := range activity.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, dto.AgentToolCallDTO{
			ID: c.ID, TurnID: c.TurnID, Seq: c.Seq, Name: c.Name, Target: c.Target,
			Status: c.Status, DurationMS: c.DurationMS,
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
