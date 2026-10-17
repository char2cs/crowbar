package migrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/testutil"
)

var ledgerEpoch = time.Date(2026, time.October, 17, 9, 0, 0, 0, time.UTC)

func minute(
	n int,
) time.Time {
	return ledgerEpoch.Add(time.Duration(n) * time.Minute)
}

func intPtr(
	v int,
) *int {
	return &v
}

func fixedClock() time.Time {
	return ledgerEpoch
}

func ledgerPathIn(
	tb testing.TB,
) string {
	tb.Helper()
	return filepath.Join(testutil.FixtureHome(tb).MigrateDir(), ledgerName)
}

func writeRawLedger(
	tb testing.TB,
	path string,
	content string,
) {
	tb.Helper()
	require.NoError(tb, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(tb, os.WriteFile(path, []byte(content), 0o600))
}

func readRawLedger(
	tb testing.TB,
	path string,
) string {
	tb.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(tb, err)
	return string(raw)
}

func appendRaw(
	tb testing.TB,
	path string,
	content string,
) {
	tb.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	require.NoError(tb, err)
	_, writeErr := file.WriteString(content)
	require.NoError(tb, writeErr)
	require.NoError(tb, file.Close())
}

func jsonLine(
	tb testing.TB,
	r record,
) string {
	tb.Helper()
	r.V = 1
	line, err := json.Marshal(r)
	require.NoError(tb, err)
	return string(line) + "\n"
}

func appendAll(
	tb testing.TB,
	l *ledger,
	records ...record,
) {
	tb.Helper()
	for _, r := range records {
		require.NoError(tb, l.append(r))
	}
}

func TestLedger_AppendThenReadBack(t *testing.T) {
	path := ledgerPathIn(t)
	l, err := openLedger(path)
	require.NoError(t, err)
	records := []record{
		{Kind: "run.start", At: minute(0), Run: "r1", Version: "0.9.0", From: intPtr(0), Pending: []int{1, 2}},
		{Kind: "step.start", At: minute(1), Run: "r1", Step: 1, Name: "a", Phase: "pre-open"},
		{Kind: "step.done", At: minute(2), Run: "r1", Step: 1, Name: "a", Outcome: "applied", Counts: Counts{"files": 4}},
		{Kind: "step.start", At: minute(3), Run: "r1", Step: 2, Name: "b", Phase: "pre-open"},
		{Kind: "checkpoint", At: minute(4), Run: "r1", Step: 2, Name: "b", Key: "k", Counts: Counts{"files": 2}},
		{Kind: "step.done", At: minute(5), Run: "r1", Step: 2, Name: "b", Outcome: "applied", Counts: Counts{"files": 3, "links": 1}},
		{Kind: "run.end", At: minute(6), Run: "r1", To: intPtr(1), Outcome: "completed"},
	}
	want := make([]record, len(records))
	for i, r := range records {
		want[i] = r
		want[i].V = 1
	}

	appendAll(t, l, records...)
	assert.Equal(t, want, l.view.records, "append keeps the live view current")
	require.NoError(t, l.Close())

	view, err := readLedger(path)

	require.NoError(t, err)
	assert.Equal(t, want, view.records)
	assert.True(t, view.applied(2, "b"))
	assert.False(t, view.applied(2, "other"), "applied is keyed by number AND name")
	assert.False(t, view.applied(3, "b"))
	got, ok := view.lastRun()
	require.True(t, ok)
	assert.Equal(t, RunSummary{
		ID:         "r1",
		Version:    "0.9.0",
		Outcome:    "completed",
		Start:      minute(0),
		End:        minute(6),
		LayoutFrom: 0,
		LayoutTo:   1,
		Counts:     Counts{"files": 7, "links": 1},
	}, got, "run counts sum the step.done records, not the checkpoints they already include")
}

func TestLedger_AppliedIgnoresOtherOutcomesAndKinds(t *testing.T) {
	view := ledgerView{records: []record{
		{Kind: "step.start", Step: 1, Name: "a"},
		{Kind: "step.fail", Step: 1, Name: "a", Outcome: "applied"},
		{Kind: "step.done", Step: 2, Name: "b", Outcome: "skipped"},
	}}

	assert.False(t, view.applied(1, "a"))
	assert.False(t, view.applied(2, "b"))
}

func TestLedger_TornFinalLineIsIgnoredAndTruncated(t *testing.T) {
	first := record{Kind: "run.start", At: minute(0), Run: "r1"}
	second := record{Kind: "step.start", At: minute(1), Run: "r1", Step: 1, Name: "a"}
	third := record{Kind: "step.done", At: minute(2), Run: "r1", Step: 1, Name: "a", Outcome: "applied"}
	intact := jsonLine(t, first) + jsonLine(t, second)
	tails := []struct {
		name string
		tail string
	}{
		{name: "cut mid-record", tail: `{"v":1,"kind":"ste`},
		{name: "complete record missing only its newline", tail: strings.TrimSuffix(jsonLine(t, third), "\n")},
	}
	for _, tc := range tails {
		t.Run(tc.name, func(t *testing.T) {
			path := ledgerPathIn(t)
			torn := intact + tc.tail
			writeRawLedger(t, path, torn)

			view, err := readLedger(path)
			require.NoError(t, err)
			assert.Len(t, view.records, 2)
			assert.Equal(t, torn, readRawLedger(t, path), "the read path never repairs")

			l, err := openLedger(path)
			require.NoError(t, err)
			assert.Len(t, l.view.records, 2)
			require.NoError(t, l.append(third))
			require.NoError(t, l.Close())

			content := readRawLedger(t, path)
			assert.Equal(t, intact+jsonLine(t, third), content)
			for _, line := range strings.Split(strings.TrimSuffix(content, "\n"), "\n") {
				assert.True(t, json.Valid([]byte(line)), "every line parses: %q", line)
			}
		})
	}
}

func TestLedger_TornOnlyLineLeavesAnEmptyLedger(t *testing.T) {
	path := ledgerPathIn(t)
	writeRawLedger(t, path, `{"v":1,"kind":"run.st`)

	l, err := openLedger(path)

	require.NoError(t, err)
	assert.Empty(t, l.view.records)
	require.NoError(t, l.Close())
	assert.Empty(t, readRawLedger(t, path))
}

func TestLedger_CorruptLinesAreAnError(t *testing.T) {
	good := jsonLine(t, record{Kind: "run.start", At: minute(0), Run: "r1"})
	cases := []struct {
		name    string
		content string
	}{
		{name: "garbage between good lines", content: good + "garbage\n" + good},
		{name: "garbage final line that ends in a newline", content: good + "garbage\n"},
		{name: "blank line between good lines", content: good + "\n" + good},
		{name: "garbage before a torn tail", content: good + "garbage\n" + `{"v":1,"ki`},
		{name: "terminated line with no kind", content: good + "{}\n"},
		{name: "terminated null line", content: good + "null\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := ledgerPathIn(t)
			writeRawLedger(t, path, tc.content)

			_, readErr := readLedger(path)
			l, openErr := openLedger(path)

			require.ErrorIs(t, readErr, ErrLedgerCorrupt)
			require.ErrorIs(t, openErr, ErrLedgerCorrupt)
			assert.Nil(t, l)
			assert.Equal(t, tc.content, readRawLedger(t, path), "corruption is reported, never repaired")
		})
	}
}

func TestLedger_DanglingRun(t *testing.T) {
	start := func(run string) record { return record{Kind: "run.start", Run: run} }
	end := func(run string) record { return record{Kind: "run.end", Run: run, Outcome: "completed"} }
	cases := []struct {
		name    string
		records []record
		wantID  string
		wantOK  bool
	}{
		{name: "no records"},
		{name: "run.start without run.end", records: []record{start("r1")}, wantID: "r1", wantOK: true},
		{name: "ended run", records: []record{start("r1"), end("r1")}},
		{name: "only the last run counts", records: []record{start("r1"), end("r1"), start("r2")}, wantID: "r2", wantOK: true},
		{name: "last run ended after an interrupted one", records: []record{start("r1"), start("r2"), end("r2")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, ok := ledgerView{records: tc.records}.danglingRun()

			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantID, id)
		})
	}
}

func TestLedger_LastRun(t *testing.T) {
	t.Run("no records", func(t *testing.T) {
		_, ok := ledgerView{}.lastRun()

		assert.False(t, ok)
	})

	t.Run("a run that has not ended reports no outcome", func(t *testing.T) {
		view := ledgerView{records: []record{
			{Kind: "run.start", At: minute(0), Run: "r1", Version: "v", From: intPtr(1)},
			{Kind: "step.done", Run: "r1", Step: 1, Name: "a", Outcome: "applied", Counts: Counts{"n": 1}},
		}}

		got, ok := view.lastRun()

		require.True(t, ok)
		assert.Equal(t, RunSummary{ID: "r1", Version: "v", Start: minute(0), LayoutFrom: 1, Counts: Counts{"n": 1}}, got)
	})

	t.Run("the latest run wins and carries its failure", func(t *testing.T) {
		view := ledgerView{records: []record{
			{Kind: "run.start", At: minute(0), Run: "r1", From: intPtr(0)},
			{Kind: "step.done", Run: "r1", Counts: Counts{"n": 5}},
			{Kind: "run.end", At: minute(1), Run: "r1", To: intPtr(1), Outcome: "completed"},
			{Kind: "run.start", At: minute(2), Run: "r2", From: intPtr(1)},
			{Kind: "step.done", Run: "r2", Counts: Counts{"n": 2}},
			{Kind: "run.end", At: minute(3), Run: "r2", To: intPtr(1), Outcome: "failed", Error: "boom"},
		}}

		got, ok := view.lastRun()

		require.True(t, ok)
		assert.Equal(t, RunSummary{
			ID: "r2", Outcome: "failed", Error: "boom",
			Start: minute(2), End: minute(3), LayoutFrom: 1, LayoutTo: 1, Counts: Counts{"n": 2},
		}, got)
	})
}

func TestLedger_CheckpointsSumPerKeyAcrossRuns(t *testing.T) {
	view := ledgerView{records: []record{
		{Kind: "checkpoint", Run: "r1", Step: 1, Name: "one", Key: "a", Counts: Counts{"x": 1}},
		{Kind: "checkpoint", Run: "r2", Step: 1, Name: "one", Key: "a", Counts: Counts{"x": 2}},
		{Kind: "checkpoint", Run: "r2", Step: 1, Name: "one", Key: "b", Counts: Counts{"y": 5}},
		{Kind: "checkpoint", Run: "r2", Step: 2, Name: "two", Key: "a", Counts: Counts{"x": 9}},
		{Kind: "step.done", Run: "r2", Step: 1, Name: "one", Counts: Counts{"x": 100}},
	}}

	assert.Equal(t, map[string]Counts{"a": {"x": 3}, "b": {"y": 5}}, view.checkpoints(1, "one"))
	assert.Equal(t, map[string]Counts{"a": {"x": 9}}, view.checkpoints(2, "two"))
	assert.Empty(t, view.checkpoints(3, "three"))
}

func TestLedger_AbsentFileIsEmpty(t *testing.T) {
	path := ledgerPathIn(t)

	view, err := readLedger(path)

	require.NoError(t, err)
	assert.Empty(t, view.records)
	_, ok := view.danglingRun()
	assert.False(t, ok)
	assert.NoDirExists(t, filepath.Dir(path), "the read path creates nothing")
}

func TestLedger_ZeroByteFileIsEmpty(t *testing.T) {
	path := ledgerPathIn(t)
	writeRawLedger(t, path, "")

	view, err := readLedger(path)
	require.NoError(t, err)
	assert.Empty(t, view.records)

	l, err := openLedger(path)
	require.NoError(t, err)
	assert.Empty(t, l.view.records)
	require.NoError(t, l.append(record{Kind: "run.start", At: minute(0), Run: "r1"}))
	require.NoError(t, l.Close())
	assert.Equal(t, jsonLine(t, record{Kind: "run.start", At: minute(0), Run: "r1"}), readRawLedger(t, path))
}

func TestLedger_OpenCreatesAPrivateLedger(t *testing.T) {
	path := ledgerPathIn(t)

	l, err := openLedger(path)

	require.NoError(t, err)
	assert.Empty(t, l.view.records)
	require.NoError(t, l.append(record{Kind: "run.start", At: minute(0), Run: "r1"}))
	require.NoError(t, l.Close())
	reopened, err := openLedger(path)
	require.NoError(t, err)
	assert.Len(t, reopened.view.records, 1, "an existing ledger is read, not replaced")
	require.NoError(t, reopened.Close())
	if runtime.GOOS != "windows" {
		fileInfo, err := os.Stat(path)
		require.NoError(t, err)
		assert.Zero(t, fileInfo.Mode().Perm()&^0o600)
		dirInfo, err := os.Stat(filepath.Dir(path))
		require.NoError(t, err)
		assert.Zero(t, dirInfo.Mode().Perm()&^0o700)
	}
}

func TestLedger_CloseIsIdempotentAndEndsAppends(t *testing.T) {
	l, err := openLedger(ledgerPathIn(t))
	require.NoError(t, err)

	require.NoError(t, l.Close())
	require.NoError(t, l.Close())

	require.ErrorContains(t, l.append(record{Kind: "run.start", Run: "r1"}), "migrate: ledger:")
	assert.Empty(t, l.view.records, "a failed append is not remembered")
}

func TestLedger_AppendStoresACopyOfTheRecord(t *testing.T) {
	l, err := openLedger(ledgerPathIn(t))
	require.NoError(t, err)
	counts, pending, from := Counts{"n": 1}, []int{1, 2}, 3

	require.NoError(t, l.append(record{Kind: "run.start", Run: "r1", Counts: counts, Pending: pending, From: &from}))
	counts["n"], pending[0], from = 99, 99, 99

	got := l.view.records[0]
	assert.Equal(t, Counts{"n": 1}, got.Counts, "the view holds what is on disk, not the caller's map")
	assert.Equal(t, []int{1, 2}, got.Pending)
	assert.Equal(t, 3, *got.From)
	require.NoError(t, l.Close())
}

func TestLedger_AppendRefusesARecordWithoutAKind(t *testing.T) {
	path := ledgerPathIn(t)
	l, err := openLedger(path)
	require.NoError(t, err)

	require.ErrorContains(t, l.append(record{Run: "r1"}), "no kind")

	assert.Empty(t, l.view.records)
	assert.Empty(t, readRawLedger(t, path), "a record that would read back as corrupt is never written")
	require.NoError(t, l.append(record{Kind: "run.start", Run: "r1"}), "a refused record does not poison the ledger")
	require.NoError(t, l.Close())
}

func TestLedger_FailedAppendPoisonsTheLedger(t *testing.T) {
	path := ledgerPathIn(t)
	start := record{Kind: "run.start", At: minute(0), Run: "r1"}
	l, err := openLedger(path)
	require.NoError(t, err)
	require.NoError(t, l.append(start))
	require.NoError(t, l.file.Close())
	require.Error(t, l.append(record{Kind: "step.start", Run: "r1"}), "writing to a dead descriptor fails")
	appendRaw(t, path, `{"v":1,"kind":"step.st`)
	before := readRawLedger(t, path)
	fresh, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	require.NoError(t, err)
	l.file = fresh

	err = l.append(record{Kind: "run.end", Run: "r1", Outcome: "failed"})

	require.Error(t, err, "a ledger whose write failed refuses every later append")
	assert.Equal(t, before, readRawLedger(t, path), "nothing may fuse onto the fragment a failed write left")
	assert.Len(t, l.view.records, 1)
	require.NoError(t, fresh.Close())

	reopened, err := openLedger(path)

	require.NoError(t, err, "the fragment is an unterminated tail, so the next boot repairs it")
	assert.Equal(t, jsonLine(t, start), readRawLedger(t, path))
	id, dangling := reopened.view.danglingRun()
	assert.True(t, dangling)
	assert.Equal(t, "r1", id)
	require.NoError(t, reopened.append(record{Kind: "run.end", Run: "r1", Outcome: "interrupted"}))
	require.NoError(t, reopened.Close())
}

func TestRecord_JSONKeysArePinned(t *testing.T) {
	full := record{
		V: 1, Kind: "backup", At: ledgerEpoch, Run: "r1", Version: "v",
		From: intPtr(0), To: intPtr(1), Pending: []int{1}, Step: 1, Name: "a", Phase: "pre-open",
		Key: "k", Counts: Counts{"n": 1}, Outcome: "applied", Error: "e", Path: "/p", Bytes: 1,
	}

	raw, err := json.Marshal(full)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))

	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	assert.ElementsMatch(t, []string{
		"v", "kind", "at", "run", "version", "layout_from", "layout_to", "pending",
		"step", "name", "phase", "key", "counts", "outcome", "error", "path", "bytes",
	}, keys)
}
