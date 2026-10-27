package chatledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

// writeLegacy writes chats into h as the old daemon kept them: WAL-mode
// databases, checkpointed on close, and one journal file per prompt.
func writeLegacy(t *testing.T, h home.Home, chats ...LegacyChat) {
	t.Helper()
	runner := writable(t, filepath.Join(storeDir(t, h), RunnerDB), ConversationsDDL, RunnersDDL)
	activity := writable(t, filepath.Join(h.Store(), ActivityDB), TurnsDDL, ToolCallsDDL, SubagentsDDL, InterruptionsDDL, ChoicesDDL)
	for _, chat := range chats {
		for _, c := range chat.Conversations {
			insert(t, runner, "agent_chat_conversations", map[string]any{
				"chat_id": c.ChatID, "session_id": c.SessionID, "provider_id": c.ProviderID,
				"first_seen_at": c.FirstSeenAt, "last_active_at": c.LastActiveAt,
			})
		}
		for _, r := range chat.Runners {
			insert(t, runner, "agent_runners", map[string]any{"id": r.ID, "current_chat_id": r.ChatID, "provider_id": r.ProviderID})
		}
		for _, r := range legacyRows(chat) {
			insert(t, activity, r.table, r.cols)
		}
		journal := filepath.Join(h.Root(), "chats", chat.ChatID, PromptsDir)
		require.NoError(t, os.MkdirAll(journal, 0o700))
		for _, p := range chat.Prompts {
			raw, err := json.Marshal(p)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(journal, p.RequestID+".json"), raw, 0o600))
		}
	}
	require.NoError(t, runner.Close())
	require.NoError(t, activity.Close())
}

type legacyRow struct {
	table string
	cols  map[string]any
}

func legacyRows(chat LegacyChat) []legacyRow {
	key := func(id string) string { return chat.ChatID + "\x00" + id }
	var out []legacyRow
	for _, r := range chat.Turns {
		out = append(out, legacyRow{"agent_turns", map[string]any{
			"key": key(r.ID), "id": r.ID, "chat_id": r.ChatID, "seq": r.Seq, "display_order": r.DisplayOrder,
			"item_index": r.ItemIndex, "role": r.Role, "provider_id": r.ProviderID, "runner_id": r.RunnerID,
			"session_id": r.SessionID, "text": r.Text, "effort": r.Effort, "status": r.Status, "diff": r.Diff,
			"plan": r.Plan, "started_at": r.StartedAt, "ended_at": r.EndedAt,
		}})
	}
	for _, r := range chat.Tools {
		out = append(out, legacyRow{"agent_tool_calls", map[string]any{
			"key": key(r.ID), "id": r.ID, "turn_id": r.TurnID, "chat_id": r.ChatID, "seq": r.Seq, "name": r.Name,
			"kind": r.Kind, "locations": r.Locations, "target": r.Target, "request_ref": r.RequestRef,
			"result_ref": r.ResultRef, "diff": r.Diff, "status": r.Status, "error": r.Error, "duration_ms": r.DurationMS,
			"subagent_id": r.SubagentID, "started_at": r.StartedAt, "ended_at": r.EndedAt,
		}})
	}
	for _, r := range chat.Subagents {
		out = append(out, legacyRow{"agent_subagents", map[string]any{
			"key": key(r.ID), "id": r.ID, "turn_id": r.TurnID, "chat_id": r.ChatID, "seq": r.Seq,
			"agent_type": r.AgentType, "started_at": r.StartedAt, "ended_at": r.EndedAt, "messages": r.Messages,
		}})
	}
	for _, r := range chat.Interruptions {
		out = append(out, legacyRow{"agent_interruptions", map[string]any{
			"key": key(r.ID), "id": r.ID, "turn_id": r.TurnID, "chat_id": r.ChatID, "seq": r.Seq,
			"display_order": r.DisplayOrder, "kind": r.Kind, "detail": r.Detail, "at": r.At, "resolved_at": r.ResolvedAt,
		}})
	}
	for _, r := range chat.Choices {
		out = append(out, legacyRow{"agent_choices", map[string]any{
			"key": key(r.ID), "id": r.ID, "turn_id": r.TurnID, "chat_id": r.ChatID, "seq": r.Seq, "kind": r.Kind,
			"prompt_id": r.PromptID, "tool_id": r.ToolID, "tool_name": r.ToolName, "title": r.Title,
			"question": r.Question, "mode": r.Mode, "multi": r.Multi, "options": r.Options, "questions": r.Questions,
			"schema": r.Schema, "at": r.At, "resolved_at": r.ResolvedAt, "resolution": r.Resolution,
			"auto_approved": false, "answered_option_ids": r.AnsweredOptionIDs,
		}})
	}
	return out
}

func insert(t *testing.T, db *sql.DB, table string, cols map[string]any) {
	t.Helper()
	names := make([]string, 0, len(cols))
	args := make([]any, 0, len(cols))
	for name, v := range cols {
		if p, ok := v.(*time.Time); ok {
			if p == nil {
				continue
			}
			v = *p
		}
		names, args = append(names, name), append(args, v)
	}
	marks := strings.TrimSuffix(strings.Repeat("?, ", len(names)), ", ")
	_, err := db.ExecContext(t.Context(),
		"INSERT INTO "+table+" ("+strings.Join(names, ", ")+") VALUES ("+marks+")", args...)
	require.NoError(t, err)
}

// files is every file under dir with its size and modification time.
func files(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out[path] = fmt.Sprintf("%s %d", info.ModTime(), info.Size())
		return nil
	}))
	return out
}

func TestReadLegacy_ReadsWithoutWriting(t *testing.T) {
	h := testutil.FixtureHome(t)
	want := chatA()
	want.Runners = []LegacyRunner{{ID: "run-1", ChatID: want.ChatID, ProviderID: "claude"}}
	writeLegacy(t, h, want)
	before := files(t, h.Root())

	got, err := ReadLegacy(context.Background(), h)

	require.NoError(t, err)
	assert.Equal(t, []LegacyChat{want}, got)
	assert.Equal(t, before, files(t, h.Root()), "reading created, removed or changed no file")
	assert.NoFileExists(t, filepath.Join(h.Store(), ActivityDB+"-wal"))
	assert.NoFileExists(t, filepath.Join(h.Store(), ActivityDB+"-shm"))
}

func TestReadLegacy_AHomeWithoutLegacyRecordsIsEmpty(t *testing.T) {
	h := testutil.FixtureHome(t)

	got, err := ReadLegacy(context.Background(), h)

	require.NoError(t, err)
	assert.Empty(t, got)
	assert.NoDirExists(t, h.Store(), "reading created nothing")
}

// A journal file the reader cannot decode is named, not fatal: it is one
// record of one chat, and the step records what it skipped.
func TestReadLegacy_AnUnreadablePromptIsNamedNotFatal(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "chats/chat-z/"+PromptsDir+"/bad.json", "{not json")
	testutil.WriteFile(t, h, "chats/chat-z/"+PromptsDir+"/.prompt-request-123", "a temp file")

	got, err := ReadLegacy(context.Background(), h)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "chat-z", got[0].ChatID)
	assert.Empty(t, got[0].Prompts)
	require.Len(t, got[0].Unreadable, 1)
	assert.Contains(t, got[0].Unreadable[0], "prompt bad.json: unreadable")
	assert.Contains(t, convert(t, got[0], nobody).Skipped, got[0].Unreadable[0], "the conversion names it as skipped")
}
