package chatledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
)

const converterVersion = 6

const importAttempts = 3

const legacyRecords = "legacy_records"

func skippedKey(chatID string) string { return "skipped/" + chatID }

func changedKey(chatID string) string { return "changed/" + chatID }

type duty int

const (
	dutyNone duty = iota
	dutyConvert
	dutyCatchUp
)

type recorded struct {
	marked func(key string) (migrate.Counts, bool)
	noted  func(key string) (migrate.Note, bool)
}

func (r recorded) owed(chatID string, n int64) duty {
	counts, converted := r.marked(chatID)
	if !converted {
		if note, skipped := r.noted(skippedKey(chatID)); skipped && note.Counts["converter"] == converterVersion {
			return dutyNone
		}
		return dutyConvert
	}
	known := counts[legacyRecords]
	note, noted := r.noted(changedKey(chatID))
	if known == 0 || n == known || noted && note.Counts[legacyRecords] == n {
		return dutyNone
	}
	return dutyCatchUp
}

func Plan(ctx context.Context, env migrate.Env) (migrate.Plan, error) {
	stamp, err := legacyStamp(env.Home)
	if err != nil || stamp == "" {
		return migrate.Plan{}, err
	}
	note, ok := env.Notes[stampKey]
	seen := ok && note.Detail == stamp && note.Counts[retryingChats] == 0
	if seen {
		return migrate.Plan{}, nil
	}
	counts, err := census(ctx, env.Home)
	if err != nil {
		return migrate.Plan{}, err
	}
	r := recorded{
		marked: func(key string) (migrate.Counts, bool) { c, ok := env.Earlier[key]; return c, ok },
		noted:  func(key string) (migrate.Note, bool) { n, ok := env.Notes[key]; return n, ok },
	}
	owed := map[duty]int64{}
	for chatID, n := range counts {
		owed[r.owed(chatID, n)]++
	}
	var plan migrate.Plan
	ranBefore := len(env.Earlier)+len(env.Notes) > 0
	if n := owed[dutyConvert]; n > 0 {
		plan.Actions = append(plan.Actions, migrate.Action{Kind: migrate.Convert, Subject: "legacy chats → chat ledger", Count: n})
		plan.RewritesState = !ranBefore
		if plan.RewritesState {
			plan.BacksUp = []string{"chats/*/" + PromptsDir}
		}
	}
	if n := owed[dutyCatchUp]; n > 0 {
		plan.Actions = append(plan.Actions, migrate.Action{
			Kind: migrate.Convert, Subject: "converted chats an older build added to", Count: n,
		})
	}
	if plan.Empty() && ranBefore && !seen {
		plan.Actions = []migrate.Action{{Kind: migrate.Record, Subject: "legacy files changed since the last run", Count: 1}}
	}
	return plan, nil
}

func Settled(marks migrate.Marks, notes map[string]migrate.Note) bool {
	stamp, ran := notes[stampKey]
	if !ran || stamp.Counts[retryingChats] > 0 {
		return false
	}
	for key := range notes {
		chatID, skipped := strings.CutPrefix(key, skippedKey(""))
		if skipped && !marks.Done(chatID) {
			return false
		}
	}
	return true
}

const retryingChats = "retrying_chats"

const stampKey = "legacy-files"

func legacyStamp(h home.Home) (string, error) {
	paths := []string{
		filepath.Join(h.Store(), ActivityDB), filepath.Join(h.Store(), ActivityDB+"-wal"),
		filepath.Join(h.Store(), RunnerDB), filepath.Join(h.Store(), RunnerDB+"-wal"),
	}
	journals, err := filepath.Glob(filepath.Join(h.Root(), "chats", "*", PromptsDir))
	if err != nil {
		return "", fmt.Errorf("chatledger: plan: %w", err)
	}
	var b strings.Builder
	found := false
	fmt.Fprintf(&b, "converter %d\n", converterVersion)
	for _, path := range append(paths, journals...) {
		info, err := os.Stat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("chatledger: plan: %w", err)
		}
		rel, err := filepath.Rel(h.Root(), path)
		if err != nil {
			return "", fmt.Errorf("chatledger: plan: %w", err)
		}
		fmt.Fprintf(&b, "%s %d %d\n", rel, info.Size(), info.ModTime().UnixNano())
		found = true
	}
	if !found {
		return "", nil
	}
	return b.String(), nil
}

func census(ctx context.Context, h home.Home) (map[string]int64, error) {
	out := map[string]int64{}
	count := func(rows *sql.Rows) error {
		var chatID string
		var n int64
		if err := rows.Scan(&chatID, &n); err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		out[chatID] += n
		return nil
	}
	for _, q := range []struct{ db, table string }{
		{RunnerDB, "agent_chat_conversations"},
		{ActivityDB, "agent_turns"},
		{ActivityDB, "agent_tool_calls"},
		{ActivityDB, "agent_subagents"},
		{ActivityDB, "agent_interruptions"},
		{ActivityDB, "agent_choices"},
	} {
		query := func(map[string]bool) string {
			return "SELECT chat_id, count(*) FROM " + q.table + " WHERE chat_id != '' GROUP BY chat_id"
		}
		if err := queryLegacy(ctx, filepath.Join(h.Store(), q.db), q.table, count, query); err != nil {
			return nil, err
		}
	}
	journals, err := filepath.Glob(filepath.Join(h.Root(), "chats", "*", PromptsDir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("chatledger: census: %w", err)
	}
	for _, path := range journals {
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
			out[filepath.Base(filepath.Dir(filepath.Dir(path)))]++
		}
	}
	return out, nil
}
