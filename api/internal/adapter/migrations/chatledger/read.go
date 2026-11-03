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
	"strconv"
	"time"
	"unicode/utf8"

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
	c, err := scanCells(rows, 5)
	if err != nil {
		return err
	}
	v := LegacyConversation{ChatID: c.str(), ProviderID: c.str(), SessionID: c.str()}
	v.FirstSeenAt, v.LastActiveAt = c.time("first_seen_at"), c.time("last_active_at")
	keep := func(chat *LegacyChat) { chat.Conversations = append(chat.Conversations, v) }
	r.add(v.ChatID, "agent_chat_conversations "+v.SessionID, c.err, keep)
	// Its times only place the binding, which its turns place as well: an
	// unreadable one is named and counted, and the session kept.
	if c.err != nil {
		keep(r.chat(v.ChatID))
	}
	return nil
}

func (r *reader) turn(rows *sql.Rows) error {
	c, err := scanCells(rows, 16)
	if err != nil {
		return err
	}
	t := LegacyTurn{
		ID: c.str(), ChatID: c.str(), Role: c.str(), ProviderID: c.str(), RunnerID: c.str(), SessionID: c.str(),
		Text: c.str(), Effort: c.str(), Status: c.str(), Diff: c.str(), Plan: c.str(),
		Seq: c.int("seq"), DisplayOrder: c.int("display_order"), ItemIndex: int(c.int("item_index")),
		StartedAt: c.time("started_at"), EndedAt: c.optTime("ended_at"),
	}
	r.add(t.ChatID, "agent_turns "+t.ID, c.err, func(chat *LegacyChat) { chat.Turns = append(chat.Turns, t) })
	return nil
}

func (r *reader) tool(rows *sql.Rows) error {
	c, err := scanCells(rows, 17)
	if err != nil {
		return err
	}
	t := LegacyTool{
		ID: c.str(), TurnID: c.str(), ChatID: c.str(), Name: c.str(), Kind: c.str(), Locations: c.str(),
		Target: c.str(), RequestRef: c.str(), ResultRef: c.str(), Diff: c.str(), Status: c.str(), Error: c.str(),
		SubagentID: c.str(), Seq: c.int("seq"), DurationMS: int(c.int("duration_ms")),
		StartedAt: c.time("started_at"), EndedAt: c.optTime("ended_at"),
	}
	r.add(t.ChatID, "agent_tool_calls "+t.ID, c.err, func(chat *LegacyChat) { chat.Tools = append(chat.Tools, t) })
	return nil
}

func (r *reader) subagent(rows *sql.Rows) error {
	c, err := scanCells(rows, 8)
	if err != nil {
		return err
	}
	s := LegacySubagent{
		ID: c.str(), TurnID: c.str(), ChatID: c.str(), AgentType: c.str(), Messages: c.str(),
		Seq: c.int("seq"), StartedAt: c.time("started_at"), EndedAt: c.optTime("ended_at"),
	}
	r.add(s.ChatID, "agent_subagents "+s.ID, c.err, func(chat *LegacyChat) { chat.Subagents = append(chat.Subagents, s) })
	return nil
}

func (r *reader) interruption(rows *sql.Rows) error {
	c, err := scanCells(rows, 9)
	if err != nil {
		return err
	}
	i := LegacyInterruption{
		ID: c.str(), TurnID: c.str(), ChatID: c.str(), Kind: c.str(), Detail: c.str(),
		Seq: c.int("seq"), DisplayOrder: c.int("display_order"), At: c.time("at"), ResolvedAt: c.optTime("resolved_at"),
	}
	r.add(i.ChatID, "agent_interruptions "+i.ID, c.err, func(chat *LegacyChat) {
		chat.Interruptions = append(chat.Interruptions, i)
	})
	return nil
}

func (r *reader) choice(rows *sql.Rows) error {
	c, err := scanCells(rows, 19)
	if err != nil {
		return err
	}
	ch := LegacyChoice{
		ID: c.str(), TurnID: c.str(), ChatID: c.str(), Kind: c.str(), PromptID: c.str(), ToolID: c.str(),
		ToolName: c.str(), Title: c.str(), Question: c.str(), Mode: c.str(), Options: c.str(), Questions: c.str(),
		Schema: c.str(), Resolution: c.str(), AnsweredOptionIDs: c.str(), Multi: c.flag("multi"),
		Seq: c.int("seq"), At: c.time("at"), ResolvedAt: c.optTime("resolved_at"),
	}
	r.add(ch.ChatID, "agent_choices "+ch.ID, c.err, func(chat *LegacyChat) { chat.Choices = append(chat.Choices, ch) })
	return nil
}

// add files a read row under its chat, or names it in the chat's Unreadable
// when one of its values could not be read.
func (r *reader) add(chatID, row string, err error, keep func(*LegacyChat)) {
	chat := r.chat(chatID)
	if err != nil {
		chat.Unreadable = append(chat.Unreadable, clip(fmt.Sprintf("%s: %v", row, err), reasonBytes))
		return
	}
	keep(chat)
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

// How much of a legacy value, and of a reason naming one, the step repeats:
// a damaged cell can be any size, and its reason goes to the log and ledger.
const (
	valueBytes  = 80
	reasonBytes = 320
)

// clip is s cut to at most n bytes, on a rune boundary, saying how long it was.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return fmt.Sprintf("%s…(%d bytes)", s[:cut], len(s))
}

// cells is one legacy row read as text, then parsed column by column in
// order: a value of the wrong type fails its own row, not the whole read.
type cells struct {
	vals []sql.NullString
	next int
	err  error
}

func scanCells(rows *sql.Rows, n int) (*cells, error) {
	c := &cells{vals: make([]sql.NullString, n)}
	dest := make([]any, n)
	for i := range c.vals {
		dest[i] = &c.vals[i]
	}
	if err := rows.Scan(dest...); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return c, nil
}

func (c *cells) take() sql.NullString {
	v := c.vals[c.next]
	c.next++
	return v
}

func (c *cells) str() string { return c.take().String }

// fail names the column, its value's first bytes and what it should have been;
// the parser's own error repeats the whole value, so it is left out.
func (c *cells) fail(column, value, want string) {
	if c.err == nil {
		c.err = fmt.Errorf("%s %q: not %s", column, clip(value, valueBytes), want)
	}
}

func (c *cells) int(column string) int64 {
	v := c.take()
	n, err := strconv.ParseInt(v.String, 10, 64)
	if err != nil {
		c.fail(column, v.String, "an integer")
	}
	return n
}

func (c *cells) flag(column string) bool {
	v := c.take()
	b, err := strconv.ParseBool(v.String)
	if err != nil {
		c.fail(column, v.String, "a boolean")
	}
	return b
}

func (c *cells) optTime(column string) *time.Time {
	v := c.take()
	if !v.Valid {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, v.String)
	if err != nil {
		c.fail(column, v.String, "an RFC 3339 time")
		return nil
	}
	t = t.UTC()
	return &t
}

// time is a timestamp; NULL is the zero time.
func (c *cells) time(column string) time.Time {
	if t := c.optTime(column); t != nil {
		return *t
	}
	return time.Time{}
}
