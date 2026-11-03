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

// converterVersion is raised by a build that converts records an older one
// refused, so the chats those builds skipped are tried again.
const converterVersion = 1

// legacyRecords is the count label under which a chat's checkpoint and notes
// keep how many legacy records it had.
const legacyRecords = "legacy_records"

// The note keys of a chat that was skipped, or changed after its conversion.
func skippedKey(chatID string) string { return "skipped/" + chatID }

func changedKey(chatID string) string { return "changed/" + chatID }

// duty is what a boot owes one legacy chat.
type duty int

const (
	dutyNone duty = iota
	dutyConvert
	dutyRecordChange
)

// recorded is what the step's earlier attempts left about the chats.
type recorded struct {
	marked func(key string) (migrate.Counts, bool)
	noted  func(key string) (migrate.Note, bool)
}

// owed decides a chat of n legacy records: convert it unless it was converted,
// or skipped by this converter; a converted chat an older build has since added to is recorded,
// once per count, and never imported over what the ledger holds.
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
	if known == 0 || n <= known || noted && note.Counts[legacyRecords] == n {
		return dutyNone
	}
	return dutyRecordChange
}

// Plan counts the legacy chats a boot still owes work. The step recurs, so
// this runs at every boot: while the legacy files are as the last run left
// them it only stats them, else it counts records, never reading one.
func Plan(ctx context.Context, env migrate.Env) (migrate.Plan, error) {
	stamp, err := legacyStamp(env.Home)
	if err != nil || stamp == "" {
		return migrate.Plan{}, err
	}
	if note, ok := env.Notes[stampKey]; ok && note.Detail == stamp {
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
		// A later run only imports chats the ledger has no row of: nothing a
		// backup would restore, so the upgrade's backup stays the only one.
		plan.RewritesState = !ranBefore
	}
	if n := owed[dutyRecordChange]; n > 0 {
		plan.Actions = append(plan.Actions, migrate.Action{
			Kind: migrate.Record, Subject: "converted chats an older build added to", Count: n,
		})
	}
	// Once a run has recorded anything, a stamp it did not see is recorded too,
	// so the next boot is back to stats alone.
	if plan.Empty() && ranBefore {
		plan.Actions = []migrate.Action{{Kind: migrate.Record, Subject: "legacy files changed since the last run", Count: 1}}
	}
	return plan, nil
}

// stampKey is the note under which a run keeps the legacyStamp it saw.
const stampKey = "legacy-files"

// legacyStamp names the converter and the size and modification time of every
// legacy database and prompt journal: an older build writing any of them, or a
// newer converter, changes it. It is "" when the home has no legacy file.
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

// census counts each chat's legacy records as ReadLegacy reads them, from the
// chat id indexes and the journal listing alone.
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
		query := "SELECT chat_id, count(*) FROM " + q.table + " WHERE chat_id != '' GROUP BY chat_id"
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
