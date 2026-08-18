package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Turns returns a chat's turns in sequence order.
//
// after and before are exclusive sequence bounds; limit caps the page. A limit of
// zero means "no cap", which only the handoff assembler asks for — every other
// caller pages.
func (s *Store) Turns(
	ctx context.Context,
	chatID string,
	after, before int64,
	limit int,
) ([]domain.ActivityTurn, error) {
	q := s.db.WithContext(ctx).Model(&TurnRow{}).Where("chat_id = ?", chatID)
	if after > 0 {
		q = q.Where("seq > ?", after)
	}
	if before > 0 {
		q = q.Where("seq < ?", before)
	}
	q = q.Order("seq ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []TurnRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("agentactivity storage: turns: %w", err)
	}
	out := make([]domain.ActivityTurn, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.domain())
	}
	return out, nil
}

// TurnsBefore returns the NEWEST turns below a sequence, in ascending order.
//
// It reads descending with a limit and reverses, rather than reading ascending
// and discarding: a chat with ten thousand turns must not load ten thousand rows
// to show the last twenty.
func (s *Store) TurnsBefore(
	ctx context.Context,
	chatID string,
	before int64,
	limit int,
) ([]domain.ActivityTurn, error) {
	q := s.db.WithContext(ctx).Model(&TurnRow{}).Where("chat_id = ?", chatID)
	if before > 0 {
		q = q.Where("seq < ?", before)
	}
	var rows []TurnRow
	if err := q.Order("seq DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("agentactivity storage: turns before: %w", err)
	}
	out := make([]domain.ActivityTurn, len(rows))
	for i, r := range rows {
		out[len(rows)-1-i] = r.domain()
	}
	return out, nil
}

// TurnsSince returns the turns recorded strictly after a wall-clock instant. It
// backs the handoff gap: what happened while a provider was away.
func (s *Store) TurnsSince(
	ctx context.Context,
	chatID string,
	cut time.Time,
) ([]domain.ActivityTurn, error) {
	var rows []TurnRow
	err := s.db.WithContext(ctx).Model(&TurnRow{}).
		Where("chat_id = ? AND started_at > ?", chatID, cut).
		Order("seq ASC").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("agentactivity storage: turns since: %w", err)
	}
	out := make([]domain.ActivityTurn, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.domain())
	}
	return out, nil
}

// CountTurns reports how many turns a chat holds.
func (s *Store) CountTurns(ctx context.Context, chatID string) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&TurnRow{}).
		Where("chat_id = ?", chatID).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("agentactivity storage: count turns: %w", err)
	}
	return count, nil
}

// LastTurnAt returns when a provider last spoke in a chat, and whether it ever
// did. It selects which native session a resume should target.
func (s *Store) LastTurnAt(
	ctx context.Context,
	chatID, providerID string,
) (time.Time, bool, error) {
	var row TurnRow
	err := s.db.WithContext(ctx).Model(&TurnRow{}).
		Where("chat_id = ? AND provider_id = ?", chatID, providerID).
		Order("seq DESC").Limit(1).Take(&row).Error
	if err != nil {
		return time.Time{}, false, nil //nolint:nilerr // absence is not a failure here
	}
	return row.StartedAt, true, nil
}

// LastTurnForSession returns when a specific native session last recorded a turn.
func (s *Store) LastTurnForSession(
	ctx context.Context,
	chatID, providerID, sessionID string,
) (time.Time, bool, error) {
	var row TurnRow
	err := s.db.WithContext(ctx).Model(&TurnRow{}).
		Where("chat_id = ? AND provider_id = ? AND session_id = ?", chatID, providerID, sessionID).
		Order("seq DESC").Limit(1).Take(&row).Error
	if err != nil {
		return time.Time{}, false, nil //nolint:nilerr // absence is not a failure here
	}
	return row.StartedAt, true, nil
}

// HasTurnAtOrAfter reports whether a provider recorded anything at or after an
// instant — the "did this CLI actually say something in its own session" check.
func (s *Store) HasTurnAtOrAfter(
	ctx context.Context,
	chatID, providerID string,
	since time.Time,
) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&TurnRow{}).
		Where("chat_id = ? AND provider_id = ? AND started_at >= ?", chatID, providerID, since).
		Limit(1).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("agentactivity storage: has turn at or after: %w", err)
	}
	return count > 0, nil
}

// ToolCalls returns a chat's tool calls in sequence order, newest last.
func (s *Store) ToolCalls(
	ctx context.Context,
	chatID string,
	after int64,
	limit int,
) ([]domain.ActivityToolCall, error) {
	q := s.db.WithContext(ctx).Model(&ToolCallRow{}).Where("chat_id = ?", chatID)
	if after > 0 {
		q = q.Where("seq > ?", after)
	}
	q = q.Order("seq ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []ToolCallRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("agentactivity storage: tool calls: %w", err)
	}
	out := make([]domain.ActivityToolCall, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.domain())
	}
	return out, nil
}

// Subagents returns a chat's subagent records in sequence order.
func (s *Store) Subagents(ctx context.Context, chatID string) ([]domain.ActivitySubagent, error) {
	var rows []SubagentRow
	err := s.db.WithContext(ctx).Model(&SubagentRow{}).
		Where("chat_id = ?", chatID).Order("seq ASC").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("agentactivity storage: subagents: %w", err)
	}
	out := make([]domain.ActivitySubagent, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.domain())
	}
	return out, nil
}

// Interruptions returns a chat's interruption records in sequence order.
func (s *Store) Interruptions(
	ctx context.Context,
	chatID string,
) ([]domain.ActivityInterruption, error) {
	var rows []InterruptionRow
	err := s.db.WithContext(ctx).Model(&InterruptionRow{}).
		Where("chat_id = ?", chatID).Order("seq ASC").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("agentactivity storage: interruptions: %w", err)
	}
	out := make([]domain.ActivityInterruption, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.domain())
	}
	return out, nil
}

// Choices returns a chat's prompts in sequence order, pending and resolved alike.
func (s *Store) Choices(ctx context.Context, chatID string) ([]domain.ActivityChoice, error) {
	var rows []ChoiceRow
	err := s.db.WithContext(ctx).Model(&ChoiceRow{}).
		Where("chat_id = ?", chatID).Order("seq ASC").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("agentactivity storage: choices: %w", err)
	}
	return choiceDomains(rows), nil
}

// PendingChoices returns only the prompts a chat is still waiting on.
//
// It is a query of its own rather than a filter over Choices because it answers
// the one question a chat surface asks on every frame — is this agent blocked on
// me — and answering it must not read a turn's worth of resolved history.
func (s *Store) PendingChoices(
	ctx context.Context,
	chatID string,
) ([]domain.ActivityChoice, error) {
	var rows []ChoiceRow
	err := s.db.WithContext(ctx).Model(&ChoiceRow{}).
		Where("chat_id = ? AND resolved_at IS NULL", chatID).
		Order("seq ASC").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("agentactivity storage: pending choices: %w", err)
	}
	return choiceDomains(rows), nil
}

func choiceDomains(rows []ChoiceRow) []domain.ActivityChoice {
	out := make([]domain.ActivityChoice, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.domain())
	}
	return out
}

// RecentToolCalls returns the most recent tool calls across a set of chats. It
// backs the cross-agent MCP surface: what are the other agents in this workspace
// touching right now.
func (s *Store) RecentToolCalls(
	ctx context.Context,
	chatIDs []string,
	since time.Time,
	limit int,
) ([]domain.ActivityToolCall, error) {
	if len(chatIDs) == 0 {
		return nil, nil
	}
	q := s.db.WithContext(ctx).Model(&ToolCallRow{}).
		Where("chat_id IN ? AND started_at >= ?", chatIDs, since).
		Order("started_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []ToolCallRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("agentactivity storage: recent tool calls: %w", err)
	}
	out := make([]domain.ActivityToolCall, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.domain())
	}
	return out, nil
}

func (r TurnRow) domain() domain.ActivityTurn {
	return domain.ActivityTurn{
		ID: r.ID, ChatID: r.ChatID, Seq: r.Seq, Role: r.Role,
		ProviderID: r.ProviderID, RunnerID: r.RunnerID, SessionID: r.SessionID,
		Text: r.Text, Effort: r.Effort, StartedAt: r.StartedAt, EndedAt: r.EndedAt,
	}
}

func (r ToolCallRow) domain() domain.ActivityToolCall {
	return domain.ActivityToolCall{
		ID: r.ID, TurnID: r.TurnID, ChatID: r.ChatID, Seq: r.Seq,
		Name: r.Name, Target: r.Target, RequestRef: r.RequestRef, ResultRef: r.ResultRef,
		Status: r.Status, Error: r.Error, DurationMS: r.DurationMS,
		StartedAt: r.StartedAt, EndedAt: r.EndedAt,
	}
}

func (r SubagentRow) domain() domain.ActivitySubagent {
	return domain.ActivitySubagent{
		ID: r.ID, TurnID: r.TurnID, ChatID: r.ChatID, Seq: r.Seq,
		AgentType: r.AgentType, StartedAt: r.StartedAt, EndedAt: r.EndedAt,
	}
}

func (r InterruptionRow) domain() domain.ActivityInterruption {
	return domain.ActivityInterruption{
		ID: r.ID, TurnID: r.TurnID, ChatID: r.ChatID, Seq: r.Seq,
		Kind: r.Kind, Detail: r.Detail, At: r.At, ResolvedAt: r.ResolvedAt,
	}
}

func (r ChoiceRow) domain() domain.ActivityChoice {
	return domain.ActivityChoice{
		ID: r.ID, TurnID: r.TurnID, ChatID: r.ChatID, Seq: r.Seq,
		Kind: r.Kind, PromptID: r.PromptID, ToolID: r.ToolID, ToolName: r.ToolName,
		Title: r.Title, Question: r.Question, Mode: r.Mode, Multi: r.Multi,
		Options: decodeOptions(r.Options), Schema: r.Schema,
		At: r.At, ResolvedAt: r.ResolvedAt, Resolution: r.Resolution,
	}
}

// decodeOptions reads the stored option list, answering "no options" for anything
// it cannot parse.
//
// A prompt whose options were written by a future shape of this code must still
// render as the question it is: losing the buttons is a degraded prompt, while
// failing the read would lose the whole timeline that holds it.
func decodeOptions(raw string) []domain.ActivityChoiceOption {
	if raw == "" {
		return nil
	}
	var out []domain.ActivityChoiceOption
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}
