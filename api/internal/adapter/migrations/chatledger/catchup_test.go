package chatledger

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

// later is what an older build, run on the upgraded home, records in chat-a
// after the conversion: a prompt and its answer, an hour on.
func later() LegacyChat {
	turn := func(id, role string, seq, order int64, start time.Time, end *time.Time, text string) LegacyTurn {
		return LegacyTurn{
			ID: id, ChatID: "chat-a", Role: role, ProviderID: "claude", RunnerID: "run-9", SessionID: "S-old",
			Text: text, Seq: seq, DisplayOrder: order, StartedAt: start, EndedAt: end,
		}
	}
	return LegacyChat{ChatID: "chat-a", Turns: []LegacyTurn{
		turn("u-late", roleUser, 98, 5, at(12, 0, 0), nil, "typed on the older build"),
		turn("msg-late", roleAssistant, 99, 5, at(12, 1, 0), ptr(at(12, 1, 30)), "answered on the older build"),
	}}
}

// both is chat-a as the legacy files hold it once later() is added.
func both() LegacyChat {
	all := chatA()
	all.Turns = append(all.Turns, later().Turns...)
	return all
}

// Turns an older build added to a converted chat are imported after the rows
// the conversion wrote, as converting the whole chat would have placed them,
// once: the next boot finds nothing owed and writes nothing.
func TestBoot_TurnsAnOlderBuildAddedToAConvertedChatAreImported(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA())
	boot(t, h)
	writeLegacy(t, h, later())

	boot(t, h)

	assert.Equal(t, convert(t, both(), nobody).Rows, held(t, h, "chat-a"))
	before := testutil.TreeSnapshot(t, h)
	boot(t, h)
	assert.Equal(t, before, testutil.TreeSnapshot(t, h))
}

// A boot stopped after the catch-up landed but before it was noted finds the
// rows already there: it adds none twice.
func TestBoot_ACatchUpThatLandedUnnotedIsNotImportedTwice(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA())
	boot(t, h)
	writeLegacy(t, h, later())
	want := convert(t, both(), nobody).Rows
	head := int64(len(held(t, h, "chat-a")))
	require.NoError(t, withStore(t.Context(), h, func(s *ledgerstore.Store) error {
		return s.ImportAfter(context.Background(), "chat-a", head, want[head:])
	}))

	boot(t, h)

	assert.Equal(t, want, held(t, h, "chat-a"))
}

// A chat this build has since written to holds rows the legacy files never
// will: its additions are noted and warned of once, never merged into it.
func TestBoot_AChatBothBuildsAddedToKeepsItsLedgerAndIsRecordedOnce(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA())
	boot(t, h)
	marker, err := ledger.NewRow(ledger.KindMarker, "", "", ledger.Marker{Kind: ledger.MarkerStopped})
	require.NoError(t, err)
	appendTo(t, h, "chat-a", marker)
	rows := held(t, h, "chat-a")
	writeLegacy(t, h, later())
	log, warned := capture(testutil.AtLeast(slog.LevelWarn))

	require.NoError(t, bootLogging(t, h, log))

	assert.Equal(t, rows, held(t, h, "chat-a"))
	require.Len(t, *warned, 1)
	attrs := testutil.Attrs((*warned)[0])
	assert.Equal(t, "chat-a", attrs["chat_id"].String())
	assert.Equal(t, records(both()), attrs["legacy_records"].Int64())
	before := testutil.TreeSnapshot(t, h)
	require.NoError(t, bootLogging(t, h, log))
	assert.Len(t, *warned, 1, "the same loss is warned of once")
	assert.Equal(t, before, testutil.TreeSnapshot(t, h))
}

// A chat an older build deleted is gone from the legacy files only: the ledger
// keeps it, and nothing is noted.
func TestBoot_AChatAnOlderBuildDeletedKeepsItsLedger(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA(), chatB())
	boot(t, h)
	rows := held(t, h, "chat-b")
	conn := writable(t, filepath.Join(h.Store(), ActivityDB))
	for _, table := range []string{"agent_turns", "agent_tool_calls", "agent_subagents", "agent_interruptions", "agent_choices"} {
		_, err := conn.ExecContext(t.Context(), "DELETE FROM "+table+" WHERE chat_id = 'chat-b'")
		require.NoError(t, err)
	}
	require.NoError(t, conn.Close())
	runner := writable(t, filepath.Join(storeDir(t, h), RunnerDB))
	_, err := runner.ExecContext(t.Context(), "DELETE FROM agent_chat_conversations WHERE chat_id = 'chat-b'")
	require.NoError(t, err)
	require.NoError(t, runner.Close())

	boot(t, h)

	assert.Equal(t, rows, held(t, h, "chat-b"))
	assert.Equal(t, convert(t, chatA(), nobody).Rows, held(t, h, "chat-a"))
}

// Legacy files retired after the upgrade, then written afresh by an older
// build, hold only what it recorded since: those turns come after the chat's
// rows, once.
func TestBoot_TurnsInLegacyFilesWrittenAfreshAreImportedAfterTheChat(t *testing.T) {
	h := testutil.FixtureHome(t)
	writeLegacy(t, h, chatA())
	boot(t, h)
	rows := held(t, h, "chat-a")
	for _, db := range []string{filepath.Join(h.Store(), ActivityDB), filepath.Join(storeDir(t, h), RunnerDB)} {
		require.NoError(t, os.Remove(db))
	}
	require.NoError(t, os.RemoveAll(filepath.Join(h.Root(), "chats")))
	writeLegacy(t, h, later())

	boot(t, h)

	got := held(t, h, "chat-a")
	require.Greater(t, len(got), len(rows))
	assert.Equal(t, rows, got[:len(rows)], "the chat's rows are kept as they were")
	added := convert(t, later(), nobody).Rows
	require.Len(t, got[len(rows):], len(added))
	for i, r := range got[len(rows):] {
		assert.Equal(t, added[i].Kind, r.Kind)
		assert.Equal(t, added[i].Body, r.Body)
	}
	before := testutil.TreeSnapshot(t, h)
	boot(t, h)
	assert.Equal(t, before, testutil.TreeSnapshot(t, h))
}
