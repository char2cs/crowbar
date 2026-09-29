package handlers

import (
	"sort"
	"time"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func activityComponentDTOs(in dto.AgentActivityDTO, turns []domain.ActivityTurn) []dto.AgentActivityComponentDTO {
	out := make([]dto.AgentActivityComponentDTO, 0,
		len(turns)+len(in.ToolCalls)+len(in.Subagents)+len(in.Interruptions)+len(in.Choices))
	subagentTurns := make(map[string]string, len(in.Subagents))
	for _, subagent := range in.Subagents {
		if subagent.ID != "" && subagent.TurnID != "" {
			subagentTurns[subagent.ID] = subagent.TurnID
		}
	}
	for _, turn := range turns {
		out = append(out, turnComponents(turn)...)
	}
	for _, tool := range in.ToolCalls {
		out = append(out, toolComponents(tool, subagentTurns)...)
	}
	for _, subagent := range in.Subagents {
		out = append(out, subagentComponent(subagent))
	}
	for _, choice := range in.Choices {
		out = append(out, choiceComponent(choice))
	}
	for _, interruption := range in.Interruptions {
		out = append(out, interruptionComponent(interruption))
	}
	sortComponents(out)
	return out
}

func turnComponents(turn domain.ActivityTurn) []dto.AgentActivityComponentDTO {
	kind := "assistant_message"
	switch turn.Role {
	case domain.TurnRoleUser:
		kind = "user_message"
	case domain.TurnRoleNotice:
		kind = "status_notice"
	}
	status, updated := activityTurnComponentStatus(turn), turn.StartedAt
	if turn.EndedAt != nil {
		updated = *turn.EndedAt
	}
	out := []dto.AgentActivityComponentDTO{{
		ID: turn.ID, TurnID: turn.ID, Seq: turn.Seq, Kind: kind,
		Status: status, CreatedAt: turn.StartedAt, UpdatedAt: updated,
		CompletedAt: turn.EndedAt,
		Payload: map[string]any{
			"text": turn.Text, "providerId": turn.ProviderID, "effort": turn.Effort,
		},
		Updates: lifecycleComponentUpdates(turn.ID, turn.StartedAt, turn.EndedAt, "active", status,
			map[string]any{"text": turn.Text, "effort": turn.Effort}),
	}}
	if len(turn.Plan) > 0 {
		out = append(out, planComponent(turn, status, updated))
	}
	if turn.Diff != "" {
		out = append(out, diffComponent(turn, updated))
	}
	return out
}

func planComponent(turn domain.ActivityTurn, status string, updated time.Time) dto.AgentActivityComponentDTO {
	planID := turn.ID + ":plan"
	planAt := turn.StartedAt
	if turn.PlanUpdatedAt != nil {
		planAt = *turn.PlanUpdatedAt
	}
	planPayload := map[string]any{"steps": turn.Plan}
	planUpdates := []dto.AgentActivityComponentUpdateDTO{{
		ID: planID + ":1", Seq: 1, Kind: "snapshot", At: planAt,
		Status: "active", Payload: planPayload,
	}}
	if turn.EndedAt != nil {
		planUpdates = append(planUpdates, dto.AgentActivityComponentUpdateDTO{
			ID: planID + ":2", Seq: 2, Kind: "status_changed", At: *turn.EndedAt,
			Status: status, Payload: map[string]any{},
		})
	}
	return dto.AgentActivityComponentDTO{
		ID: planID, TurnID: turn.ID, ParentID: turn.ID, Seq: turn.Seq,
		Kind: "plan", Status: status, CreatedAt: planAt,
		UpdatedAt: updated, CompletedAt: turn.EndedAt,
		Payload: planPayload, Updates: planUpdates,
	}
}

func diffComponent(turn domain.ActivityTurn, updated time.Time) dto.AgentActivityComponentDTO {
	diffID := turn.ID + ":diff"
	diffPayload := map[string]any{"unifiedDiff": turn.Diff}
	return dto.AgentActivityComponentDTO{
		ID: diffID, TurnID: turn.ID, ParentID: turn.ID, Seq: turn.Seq,
		Kind: "diff", Status: "completed", CreatedAt: updated,
		UpdatedAt: updated, CompletedAt: turn.EndedAt,
		Payload: diffPayload,
		Updates: []dto.AgentActivityComponentUpdateDTO{{
			ID: diffID + ":1", Seq: 1, Kind: "snapshot", At: updated,
			Status: "completed", Payload: diffPayload,
		}},
	}
}

func toolComponentStatus(status string) string {
	switch status {
	case domain.ToolStatusRunning:
		return "active"
	case domain.ToolStatusError:
		return "failed"
	case domain.ToolStatusDeclined:
		return "declined"
	case domain.ToolStatusAbandoned:
		return "abandoned"
	}
	return "completed"
}

func toolComponents(tool dto.AgentToolCallDTO, subagentTurns map[string]string) []dto.AgentActivityComponentDTO {
	status := toolComponentStatus(tool.Status)
	updated := tool.StartedAt
	if tool.EndedAt != nil {
		updated = *tool.EndedAt
	}
	turnID := tool.TurnID
	if turnID == "" && tool.SubagentID != "" {
		turnID = subagentTurns[tool.SubagentID]
	}
	out := []dto.AgentActivityComponentDTO{{
		ID: tool.ID, TurnID: turnID, ParentID: tool.SubagentID, Seq: tool.Seq,
		Kind: "tool_call", Status: status, CreatedAt: tool.StartedAt,
		UpdatedAt: updated, CompletedAt: tool.EndedAt,
		Payload: map[string]any{
			"name": tool.Name, "kind": tool.Kind, "locations": tool.Locations,
			"target": tool.Target, "error": tool.Error,
			"durationMs": tool.DurationMS, "hasRequest": tool.HasRequest,
			"hasResult": tool.HasResult,
		},
		Updates: lifecycleComponentUpdates(tool.ID, tool.StartedAt, tool.EndedAt, "active", status,
			map[string]any{"error": tool.Error, "durationMs": tool.DurationMS, "hasResult": tool.HasResult}),
	}}
	if !tool.HasResult {
		return out
	}
	outputID := tool.ID + ":output"
	outputPayload := map[string]any{"toolCallId": tool.ID, "side": "result"}
	return append(out, dto.AgentActivityComponentDTO{
		ID: outputID, TurnID: turnID, ParentID: tool.ID, Seq: tool.Seq,
		Kind: "tool_output", Status: status, CreatedAt: tool.StartedAt,
		UpdatedAt: updated, CompletedAt: tool.EndedAt, Payload: outputPayload,
		Updates: []dto.AgentActivityComponentUpdateDTO{{
			ID: outputID + ":1", Seq: 1, Kind: "available", At: updated,
			Status: status, Payload: outputPayload,
		}},
	})
}

func subagentComponent(subagent dto.AgentSubagentDTO) dto.AgentActivityComponentDTO {
	status, updated := "active", subagent.StartedAt
	if subagent.EndedAt != nil {
		status, updated = "completed", *subagent.EndedAt
	}
	payload := map[string]any{}
	if subagent.AgentType != "" {
		payload["agentType"] = subagent.AgentType
	}
	if len(subagent.Messages) > 0 {
		payload["messages"] = subagent.Messages
	}
	return dto.AgentActivityComponentDTO{
		ID: subagent.ID, TurnID: subagent.TurnID, Seq: subagent.Seq,
		Kind: "subagent", Status: status, CreatedAt: subagent.StartedAt,
		UpdatedAt: updated, CompletedAt: subagent.EndedAt,
		Payload: payload,
		Updates: lifecycleComponentUpdates(subagent.ID, subagent.StartedAt, subagent.EndedAt, "active", status,
			map[string]any{"messages": subagent.Messages}),
	}
}

func choiceComponent(choice dto.AgentChoiceDTO) dto.AgentActivityComponentDTO {
	kind := "user_input_request"
	if choice.Kind == domain.ChoiceKindPermission {
		kind = "permission_request"
	}
	status, updated := choiceComponentStatus(choice), choice.At
	if choice.ResolvedAt != nil {
		updated = *choice.ResolvedAt
	}
	return dto.AgentActivityComponentDTO{
		ID: choice.ID, TurnID: choice.TurnID, Seq: choice.Seq, Kind: kind,
		Status: status, CreatedAt: choice.At, UpdatedAt: updated,
		CompletedAt: choice.ResolvedAt, Payload: map[string]any{"choice": choice},
		Updates: lifecycleComponentUpdates(choice.ID, choice.At, choice.ResolvedAt, "pending", status,
			map[string]any{"resolution": choice.Resolution, "answeredOptionIds": choice.AnsweredOptionIDs}),
	}
}

func interruptionComponent(interruption dto.AgentInterruptionDTO) dto.AgentActivityComponentDTO {
	status, updated := "active", interruption.At
	if interruption.ResolvedAt != nil {
		status, updated = "completed", *interruption.ResolvedAt
	}
	kind := "status_notice"
	if interruption.Kind == "compaction" {
		kind = "compaction"
	}
	return dto.AgentActivityComponentDTO{
		ID: interruption.ID, TurnID: interruption.TurnID, Seq: interruption.Seq,
		Kind: kind, Status: status, CreatedAt: interruption.At,
		UpdatedAt: updated, CompletedAt: interruption.ResolvedAt,
		Payload: map[string]any{"interruption": interruption},
		Updates: lifecycleComponentUpdates(interruption.ID, interruption.At, interruption.ResolvedAt, "active", status,
			map[string]any{"detail": interruption.Detail}),
	}
}

// sortComponents orders by seq; a parent precedes its own children.
func sortComponents(out []dto.AgentActivityComponentDTO) {
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Seq != out[j].Seq {
			return out[i].Seq < out[j].Seq
		}
		if out[i].ID == out[j].ParentID {
			return true
		}
		if out[j].ID == out[i].ParentID {
			return false
		}
		return out[i].ID < out[j].ID
	})
}
