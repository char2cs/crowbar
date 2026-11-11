package chatledger

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// dated gives each conversation with no time the chat's oldest record's, or
// when the chat has none, when the legacy files were last written.
func dated(chat LegacyChat, written time.Time) LegacyChat {
	at := cmp.Or(oldest(chat), written)
	for i, c := range chat.Conversations {
		if c.FirstSeenAt.IsZero() && c.LastActiveAt.IsZero() {
			chat.Conversations[i].FirstSeenAt = at
		}
	}
	return chat
}

// modTime is when path was last written; zero when it does not exist.
func modTime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("chatledger: read: %w", err)
	}
	return info.ModTime().UTC(), nil
}

// legacyTable is one legacy table the reader reads, column for column as its
// scan expects, a chat's rows in order.
type legacyTable struct {
	db, name string
	cols     []legacyCol
	order    string
	scan     func(*reader, *sql.Rows) error
}

// legacyCol is one column a scan reads and what it reads as when NULL (a bare
// one, as it is). A later one was added after its table: a table an older build
// created without it reads it as NULL too.
type legacyCol struct {
	name, or string
	later    bool
}

// selectList is t's columns as the table at hand holds them.
func (t legacyTable) selectList(has map[string]bool) string {
	exprs := make([]string, len(t.cols))
	for i, c := range t.cols {
		switch {
		case !has[c.name] && c.later:
			exprs[i] = cmp.Or(c.or, "NULL")
		case c.or == "":
			exprs[i] = c.name
		default:
			exprs[i] = "COALESCE(" + c.name + ", " + c.or + ")"
		}
	}
	return strings.Join(exprs, ", ")
}

// text, number and bare are a column read as ”, as 0, and as it is.
func text(name string) legacyCol   { return legacyCol{name: name, or: "''"} }
func number(name string) legacyCol { return legacyCol{name: name, or: "0"} }
func bare(name string) legacyCol   { return legacyCol{name: name} }

// added is c added to its table after the table was first created.
func added(c legacyCol) legacyCol { c.later = true; return c }

var legacyTables = []legacyTable{
	{RunnerDB, "agent_chat_conversations", []legacyCol{
		text("chat_id"), text("provider_id"), text("session_id"), bare("first_seen_at"), added(bare("last_active_at")),
	}, "first_seen_at, session_id", (*reader).conversation},
	{ActivityDB, "agent_turns", []legacyCol{
		text("id"), bare("chat_id"), text("role"), text("provider_id"), text("runner_id"), text("session_id"),
		text("text"), text("effort"), added(text("status")), added(text("diff")), added(text("plan")), number("seq"),
		number("display_order"), number("item_index"), bare("started_at"), bare("ended_at"),
	}, "seq, id", (*reader).turn},
	{ActivityDB, "agent_tool_calls", []legacyCol{
		text("id"), text("turn_id"), bare("chat_id"), text("name"), added(text("kind")), added(text("locations")), text("target"),
		text("request_ref"), text("result_ref"), added(text("diff")), text("status"), text("error"), added(text("subagent_id")),
		number("seq"), number("duration_ms"), bare("started_at"), bare("ended_at"),
	}, "seq, id", (*reader).tool},
	{ActivityDB, "agent_subagents", []legacyCol{
		text("id"), text("turn_id"), bare("chat_id"), text("agent_type"), added(text("messages")), number("seq"),
		bare("started_at"), bare("ended_at"),
	}, "seq, id", (*reader).subagent},
	{ActivityDB, "agent_interruptions", []legacyCol{
		text("id"), text("turn_id"), bare("chat_id"), text("kind"), text("detail"), number("seq"),
		number("display_order"), bare("at"), bare("resolved_at"),
	}, "seq, id", (*reader).interruption},
	{ActivityDB, "agent_choices", []legacyCol{
		text("id"), text("turn_id"), bare("chat_id"), text("kind"), text("prompt_id"), text("tool_id"),
		text("tool_name"), text("title"), text("question"), text("mode"), text("options"), text("questions"),
		text("schema"), text("resolution"), added(text("answered_option_ids")), number("multi"), number("seq"),
		bare("at"), bare("resolved_at"),
	}, "seq, id", (*reader).choice},
}

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
	chat := r.chat(v.ChatID)
	chat.Conversations = append(chat.Conversations, v)
	// Its times only place the binding, which its turns place as well: an
	// unreadable one is named, and the session kept.
	if c.err != nil {
		chat.Partial = append(chat.Partial, clip(fmt.Sprintf("agent_chat_conversations %s: %v", v.SessionID, c.err), reasonBytes))
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

// journal reads <chats>/<chat id>/prompt-requests/*.json. A record that does
// not decode is named in its chat's Unreadable, not fatal.
func (r *reader) journal(chatsDir, chatID string) error {
	journal := filepath.Join(chatsDir, chatID, PromptsDir)
	files, err := readDir(journal)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.Type().IsRegular() && filepath.Ext(f.Name()) == ".json" {
			r.prompt(chatID, filepath.Join(journal, f.Name()))
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
