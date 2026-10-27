package chatledger

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

// ReadLegacy reads every chat's records from the legacy runner and activity
// databases and the legacy prompt journal, without writing. Missing files are
// empty input; chats come back by id.
func ReadLegacy(ctx context.Context, h home.Home) ([]LegacyChat, error) {
	r := reader{chats: map[string]*LegacyChat{}}
	runnerDB, activityDB := filepath.Join(h.Store(), RunnerDB), filepath.Join(h.Store(), ActivityDB)
	for _, q := range []struct {
		path, table, query string
		scan               func(*sql.Rows) error
	}{
		{runnerDB, "agent_chat_conversations", conversationsQuery, r.conversation},
		{activityDB, "agent_turns", turnsQuery, r.turn},
		{activityDB, "agent_tool_calls", toolsQuery, r.tool},
		{activityDB, "agent_subagents", subagentsQuery, r.subagent},
		{activityDB, "agent_interruptions", interruptionsQuery, r.interruption},
		{activityDB, "agent_choices", choicesQuery, r.choice},
	} {
		if err := queryLegacy(ctx, q.path, q.table, q.scan, q.query); err != nil {
			return nil, err
		}
	}
	runners, err := LiveRunners(ctx, h)
	if err != nil {
		return nil, err
	}
	for _, run := range runners {
		c := r.chat(run.ChatID)
		c.Runners = append(c.Runners, run)
	}
	if err := r.prompts(filepath.Join(h.Root(), "chats")); err != nil {
		return nil, err
	}
	out := make([]LegacyChat, 0, len(r.chats))
	for _, id := range slices.Sorted(maps.Keys(r.chats)) {
		out = append(out, *r.chats[id])
	}
	return out, nil
}

const (
	conversationsQuery = `SELECT COALESCE(chat_id, ''), COALESCE(provider_id, ''), COALESCE(session_id, ''),
		first_seen_at, last_active_at FROM agent_chat_conversations
		WHERE COALESCE(chat_id, '') != '' ORDER BY chat_id, first_seen_at, session_id`
	turnsQuery = `SELECT COALESCE(id, ''), chat_id, COALESCE(role, ''), COALESCE(provider_id, ''),
		COALESCE(runner_id, ''), COALESCE(session_id, ''), COALESCE(text, ''), COALESCE(effort, ''),
		COALESCE(status, ''), COALESCE(diff, ''), COALESCE(plan, ''), COALESCE(seq, 0),
		COALESCE(display_order, 0), COALESCE(item_index, 0), started_at, ended_at
		FROM agent_turns WHERE COALESCE(chat_id, '') != '' ORDER BY chat_id, seq, id`
	toolsQuery = `SELECT COALESCE(id, ''), COALESCE(turn_id, ''), chat_id, COALESCE(name, ''),
		COALESCE(kind, ''), COALESCE(locations, ''), COALESCE(target, ''), COALESCE(request_ref, ''),
		COALESCE(result_ref, ''), COALESCE(diff, ''), COALESCE(status, ''), COALESCE(error, ''),
		COALESCE(subagent_id, ''), COALESCE(seq, 0), COALESCE(duration_ms, 0), started_at, ended_at
		FROM agent_tool_calls WHERE COALESCE(chat_id, '') != '' ORDER BY chat_id, seq, id`
	subagentsQuery = `SELECT COALESCE(id, ''), COALESCE(turn_id, ''), chat_id, COALESCE(agent_type, ''),
		COALESCE(messages, ''), COALESCE(seq, 0), started_at, ended_at
		FROM agent_subagents WHERE COALESCE(chat_id, '') != '' ORDER BY chat_id, seq, id`
	interruptionsQuery = `SELECT COALESCE(id, ''), COALESCE(turn_id, ''), chat_id, COALESCE(kind, ''),
		COALESCE(detail, ''), COALESCE(seq, 0), COALESCE(display_order, 0), at, resolved_at
		FROM agent_interruptions WHERE COALESCE(chat_id, '') != '' ORDER BY chat_id, seq, id`
	choicesQuery = `SELECT COALESCE(id, ''), COALESCE(turn_id, ''), chat_id, COALESCE(kind, ''),
		COALESCE(prompt_id, ''), COALESCE(tool_id, ''), COALESCE(tool_name, ''), COALESCE(title, ''),
		COALESCE(question, ''), COALESCE(mode, ''), COALESCE(options, ''), COALESCE(questions, ''),
		COALESCE(schema, ''), COALESCE(resolution, ''), COALESCE(answered_option_ids, ''),
		COALESCE(multi, 0), COALESCE(seq, 0), at, resolved_at
		FROM agent_choices WHERE COALESCE(chat_id, '') != '' ORDER BY chat_id, seq, id`
)

// reader gathers legacy records into the chats they name.
type reader struct{ chats map[string]*LegacyChat }

func (r *reader) chat(id string) *LegacyChat {
	c, ok := r.chats[id]
	if !ok {
		c = &LegacyChat{ChatID: id}
		r.chats[id] = c
	}
	return c
}

func (r *reader) conversation(rows *sql.Rows) error {
	var c LegacyConversation
	var first, last sql.NullTime
	if err := rows.Scan(&c.ChatID, &c.ProviderID, &c.SessionID, &first, &last); err != nil {
		return err
	}
	c.FirstSeenAt, c.LastActiveAt = timeOf(first), timeOf(last)
	chat := r.chat(c.ChatID)
	chat.Conversations = append(chat.Conversations, c)
	return nil
}

func (r *reader) turn(rows *sql.Rows) error {
	var t LegacyTurn
	var started, ended sql.NullTime
	if err := rows.Scan(&t.ID, &t.ChatID, &t.Role, &t.ProviderID, &t.RunnerID, &t.SessionID, &t.Text,
		&t.Effort, &t.Status, &t.Diff, &t.Plan, &t.Seq, &t.DisplayOrder, &t.ItemIndex, &started, &ended); err != nil {
		return err
	}
	t.StartedAt, t.EndedAt = timeOf(started), optTime(ended)
	chat := r.chat(t.ChatID)
	chat.Turns = append(chat.Turns, t)
	return nil
}

func (r *reader) tool(rows *sql.Rows) error {
	var t LegacyTool
	var started, ended sql.NullTime
	if err := rows.Scan(&t.ID, &t.TurnID, &t.ChatID, &t.Name, &t.Kind, &t.Locations, &t.Target, &t.RequestRef,
		&t.ResultRef, &t.Diff, &t.Status, &t.Error, &t.SubagentID, &t.Seq, &t.DurationMS, &started, &ended); err != nil {
		return err
	}
	t.StartedAt, t.EndedAt = timeOf(started), optTime(ended)
	chat := r.chat(t.ChatID)
	chat.Tools = append(chat.Tools, t)
	return nil
}

func (r *reader) subagent(rows *sql.Rows) error {
	var s LegacySubagent
	var started, ended sql.NullTime
	if err := rows.Scan(&s.ID, &s.TurnID, &s.ChatID, &s.AgentType, &s.Messages, &s.Seq, &started, &ended); err != nil {
		return err
	}
	s.StartedAt, s.EndedAt = timeOf(started), optTime(ended)
	chat := r.chat(s.ChatID)
	chat.Subagents = append(chat.Subagents, s)
	return nil
}

func (r *reader) interruption(rows *sql.Rows) error {
	var i LegacyInterruption
	var at, resolved sql.NullTime
	if err := rows.Scan(&i.ID, &i.TurnID, &i.ChatID, &i.Kind, &i.Detail, &i.Seq, &i.DisplayOrder, &at, &resolved); err != nil {
		return err
	}
	i.At, i.ResolvedAt = timeOf(at), optTime(resolved)
	chat := r.chat(i.ChatID)
	chat.Interruptions = append(chat.Interruptions, i)
	return nil
}

func (r *reader) choice(rows *sql.Rows) error {
	var c LegacyChoice
	var at, resolved sql.NullTime
	if err := rows.Scan(&c.ID, &c.TurnID, &c.ChatID, &c.Kind, &c.PromptID, &c.ToolID, &c.ToolName, &c.Title,
		&c.Question, &c.Mode, &c.Options, &c.Questions, &c.Schema, &c.Resolution, &c.AnsweredOptionIDs,
		&c.Multi, &c.Seq, &at, &resolved); err != nil {
		return err
	}
	c.At, c.ResolvedAt = timeOf(at), optTime(resolved)
	chat := r.chat(c.ChatID)
	chat.Choices = append(chat.Choices, c)
	return nil
}

// prompts reads <chats>/<chat id>/prompt-requests/*.json. A record that does
// not decode is named in its chat's Unreadable, not fatal.
func (r *reader) prompts(chatsDir string) error {
	dirs, err := readDir(chatsDir)
	if err != nil {
		return err
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		journal := filepath.Join(chatsDir, d.Name(), PromptsDir)
		files, err := readDir(journal)
		if err != nil {
			return err
		}
		for _, f := range files {
			if f.Type().IsRegular() && filepath.Ext(f.Name()) == ".json" {
				r.prompt(d.Name(), filepath.Join(journal, f.Name()))
			}
		}
	}
	return nil
}

func (r *reader) prompt(chatID, path string) {
	chat := r.chat(chatID)
	raw, err := os.ReadFile(path) //nolint:gosec // G304: path is a journal file listed under the home's own chats directory
	var p LegacyPrompt
	if err == nil {
		err = json.Unmarshal(raw, &p)
	}
	if err == nil && p.RequestID == "" {
		err = errors.New("no request id")
	}
	if err != nil {
		chat.Unreadable = append(chat.Unreadable, fmt.Sprintf("prompt %s: unreadable: %v", filepath.Base(path), err))
		return
	}
	p.CreatedAt, p.UpdatedAt = p.CreatedAt.UTC(), p.UpdatedAt.UTC()
	chat.Prompts = append(chat.Prompts, p)
	slices.SortFunc(chat.Prompts, func(a, b LegacyPrompt) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.RequestID, b.RequestID))
	})
}

// readDir lists dir; a missing one is empty.
func readDir(dir string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("chatledger: read %s: %w", dir, err)
	}
	return entries, nil
}

func timeOf(t sql.NullTime) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time.UTC()
}

func optTime(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	at := t.Time.UTC()
	return &at
}
