package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity/internal/store/internal/storage"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var now = time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)

func newStoreWithDB(t *testing.T) (context.Context, *storage.Store, *gormdb.DB) {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	st, err := storage.New(db)
	require.NoError(t, err)
	return context.Background(), st, db
}

func newStore(t *testing.T) (context.Context, *storage.Store) {
	t.Helper()
	ctx, st, _ := newStoreWithDB(t)
	return ctx, st
}

// TestTurns_BeforeFilterExcludesTheBoundarySeq pins the `before` boundary
// Turns honors alongside `after`: a turn AT the given seq sits on the far
// side of the boundary and must be excluded, the same way `after` already
// excludes its own boundary — a backward-paging read must not repeat or skip
// a row depending on which direction it walks.
func TestTurns_BeforeFilterExcludesTheBoundarySeq(t *testing.T) {
	ctx, st := newStore(t)
	for i, id := range []string{"t1", "t2", "t3"} {
		require.NoError(t, st.SaveTurn(ctx, domain.ActivityTurn{
			ID: id, ChatID: "c1", Seq: int64(i + 1), Text: id, StartedAt: now,
		}))
	}

	got, err := st.Turns(ctx, "c1", 0, 3, 0)
	require.NoError(t, err)
	ids := make([]string, 0, len(got))
	for _, turn := range got {
		ids = append(ids, turn.ID)
	}
	assert.Equal(t, []string{"t1", "t2"}, ids, "seq 3 sits AT the before boundary and must be excluded")
}

// TestToolCallsBefore_FiltersToCallsBeforeTheGivenSeq proves ToolCallsBefore's
// own `before` boundary (nothing previously exercised this method at all): a
// call whose seq the caller already holds must not be repeated when paging
// backward for the calls that came before it.
func TestToolCallsBefore_FiltersToCallsBeforeTheGivenSeq(t *testing.T) {
	ctx, st := newStore(t)
	for i, id := range []string{"a", "b", "c"} {
		require.NoError(t, st.SaveToolCall(ctx, domain.ActivityToolCall{
			ID: id, ChatID: "c1", Seq: int64(i + 1), Name: id, StartedAt: now,
		}))
	}

	got, err := st.ToolCallsBefore(ctx, "c1", 3, 10)
	require.NoError(t, err)
	names := make([]string, 0, len(got))
	for _, call := range got {
		names = append(names, call.Name)
	}
	assert.Equal(t, []string{"a", "b"}, names, "seq 3 sits AT the before boundary and must be excluded")
}

func TestToolCallsBefore_ClosedDatabaseReturnsError(t *testing.T) {
	ctx, st, db := newStoreWithDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = st.ToolCallsBefore(ctx, "c1", 0, 10)
	assert.Error(t, err)
}

// TestChoiceReads_ClosedDatabaseReturnsError proves Choices and
// PendingChoices surface a genuine read failure rather than reporting it as
// "no choices" — mirrors the closed-DB coverage the sibling
// TestStorage_QueriesReportFailureOnAClosedDatabase (projections package)
// already gives most of Store's other read methods, extended to the two
// Choice-specific reads it does not touch.
func TestChoiceReads_ClosedDatabaseReturnsError(t *testing.T) {
	ctx, st, db := newStoreWithDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = st.Choices(ctx, "c1")
	assert.Error(t, err)

	_, err = st.PendingChoices(ctx, "c1")
	assert.Error(t, err)
}

// TestResolveChoicesForTool_MatchesByNameWhenIDIsEmpty exercises the
// name-only branch of ResolveChoicesForTool's switch: a choice recorded
// against a tool NAME with no tool id yet resolved (the id is only known once
// the matching tool call itself lands) must still resolve when the tool call
// closes and reports its name.
func TestResolveChoicesForTool_MatchesByNameWhenIDIsEmpty(t *testing.T) {
	ctx, st := newStore(t)
	require.NoError(t, st.SaveChoice(ctx, domain.ActivityChoice{
		ID: "choice-1", ChatID: "c1", ToolName: "Bash", At: now,
	}))

	require.NoError(t, st.ResolveChoicesForTool(ctx, "c1", "", "Bash", &now))

	choices, err := st.Choices(ctx, "c1")
	require.NoError(t, err)
	require.Len(t, choices, 1)
	require.NotNil(t, choices[0].ResolvedAt)
	assert.Equal(t, domain.ChoiceResolutionProceeded, choices[0].Resolution)
}

// TestResolveChoicesForTool_ClosedDatabaseReturnsError and
// TestResolveOpenChoices_ClosedDatabaseReturnsError pin the write-failure
// wraps neither method's happy path (exercised via the projections package)
// ever reaches.
func TestResolveChoicesForTool_ClosedDatabaseReturnsError(t *testing.T) {
	ctx, st, db := newStoreWithDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	err = st.ResolveChoicesForTool(ctx, "c1", "tool-1", "Bash", &now)
	assert.Error(t, err)
}

func TestResolveOpenChoices_ClosedDatabaseReturnsError(t *testing.T) {
	ctx, st, db := newStoreWithDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	err = st.ResolveOpenChoices(ctx, "c1", &now)
	assert.Error(t, err)
}

// TestChoices_CorruptOptionsJSONDegradesToNilRatherThanFailing proves
// decodeList's defensive contract: a choice row whose "options" column holds
// bytes that were never written by SaveChoice's own json.Marshal (corrupt
// data from an old schema, a hand-edited row, disk corruption) must decode to
// a nil Options slice, not fail the whole Choices read.
func TestChoices_CorruptOptionsJSONDegradesToNilRatherThanFailing(t *testing.T) {
	ctx, st, db := newStoreWithDB(t)

	require.NoError(t, db.Exec(
		`INSERT INTO agent_choices (key, id, chat_id, kind, options, questions, at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"c1\x00choice-1", "choice-1", "c1", "question", "{not-valid-json", "", now,
	).Error)

	got, err := st.Choices(ctx, "c1")
	require.NoError(t, err, "corrupt embedded JSON must not fail the whole read")
	require.Len(t, got, 1)
	assert.Nil(t, got[0].Options, "malformed options JSON decodes to nil rather than propagating an error")
}
