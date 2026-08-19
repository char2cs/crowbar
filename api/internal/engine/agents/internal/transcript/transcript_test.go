package transcript_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/transcript"
)

// claudeDescriptor mirrors the transcript block claude.yaml actually ships, so a
// test failure here means the real descriptor would fail too.
func claudeDescriptor() *spec.Descriptor {
	d := &spec.Descriptor{ID: "probe"}
	d.Transcript = spec.TranscriptSpec{
		PathField: "transcript_path",
		Format:    "jsonl",
		Message: spec.TranscriptMessageSpec{
			Match:      map[string]string{"type": "assistant", "message.role": "assistant"},
			Reject:     map[string]string{"isSidechain": "true", "isApiErrorMessage": "true"},
			Blocks:     "message.content",
			BlockMatch: map[string]string{"type": "text"},
			BlockText:  "text",
			Join:       "\n\n",
			Timestamp:  "timestamp",
		},
	}
	return d
}

// codexDescriptor mirrors codex.yaml's transcript block: a different shape at
// almost every path, on purpose — it is what proves the engine itself is
// provider-blind.
func codexDescriptor() *spec.Descriptor {
	d := &spec.Descriptor{ID: "probe"}
	d.Transcript = spec.TranscriptSpec{
		PathField: "transcript_path",
		Format:    "jsonl",
		Message: spec.TranscriptMessageSpec{
			Match:      map[string]string{"type": "response_item", "payload.type": "message", "payload.role": "assistant"},
			Blocks:     "payload.content",
			BlockMatch: map[string]string{"type": "output_text"},
			BlockText:  "text",
			Join:       "\n\n",
			Timestamp:  "timestamp",
		},
	}
	return d
}

// claudeLine builds one claude-shaped transcript entry with a single text block.
func claudeLine(text, timestamp string) string {
	return fmt.Sprintf(
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":%q}]},"timestamp":%q}`,
		text, timestamp)
}

// writeLines lays down a fresh transcript file, one JSON line per entry, every
// line newline-terminated.
func writeLines(t *testing.T, path string, lines []string) {
	t.Helper()
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	require.NoError(t, os.WriteFile(path, []byte(b.String()), 0o644))
}

// appendLine simulates a live CLI process writing its next entry to a transcript
// Crowbar already has open.
func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString(line + "\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

// TestRead_ClaudeFixtureYieldsTheMessageWithItsParsedTimestamp pins the shape
// claude actually writes: one text block, read back as one message dated by the
// entry's own timestamp rather than by when Crowbar happened to read it.
func TestRead_ClaudeFixtureYieldsTheMessageWithItsParsedTimestamp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	line := claudeLine("hello there", "2026-08-18T10:00:00Z")
	writeLines(t, path, []string{line})

	got, err := transcript.Read(claudeDescriptor(), path, 0)

	require.NoError(t, err)
	require.Len(t, got.Messages, 1)
	assert.Equal(t, "hello there", got.Messages[0].Text)
	assert.False(t, got.Messages[0].Truncated)
	want, err := time.Parse(time.RFC3339, "2026-08-18T10:00:00Z")
	require.NoError(t, err)
	assert.True(t, want.Equal(got.Messages[0].At))
	assert.Equal(t, int64(len(line)+1), got.Offset)
}

// TestRead_RecognisesOnlyTheAssistantsTextBlocks is the recognition contract:
// reasoning and tool calls are the agent thinking and acting, not speaking, so
// only a text block ever becomes a message — and a user's own entry is never
// mistaken for one, however it is shaped.
func TestRead_RecognisesOnlyTheAssistantsTextBlocks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	writeLines(t, path, []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"pondering"}]},"timestamp":"2026-08-18T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{}}]},"timestamp":"2026-08-18T10:00:01Z"}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hi"}]},"timestamp":"2026-08-18T10:00:02Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"pondering"},{"type":"text","text":"the answer"}]},"timestamp":"2026-08-18T10:00:03Z"}`,
	})

	got, err := transcript.Read(claudeDescriptor(), path, 0)

	require.NoError(t, err)
	require.Len(t, got.Messages, 1, "only the fourth entry carries a text block")
	assert.Equal(t, "the answer", got.Messages[0].Text)
}

// TestRead_TwoTextBlocksInOneEntryAreJoinedIntoOneMessage: claude renders two
// text blocks of one entry as one reply, so recording them as two messages would
// misrepresent what was said.
func TestRead_TwoTextBlocksInOneEntryAreJoinedIntoOneMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	writeLines(t, path, []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"first"},{"type":"text","text":"second"}]},"timestamp":"2026-08-18T10:00:00Z"}`,
	})

	got, err := transcript.Read(claudeDescriptor(), path, 0)

	require.NoError(t, err)
	require.Len(t, got.Messages, 1)
	assert.Equal(t, "first\n\nsecond", got.Messages[0].Text)
}

// TestRead_RejectDropsSidechainAndErrorEntriesButKeepsAbsentOrExplicitNull
// exercises the one comparison Match cannot express: claude marks a subagent's
// entries and an API failure with a key that is otherwise simply not present, so
// a payload spelling that absence as an explicit JSON null must be kept exactly
// like one that omits the key altogether.
func TestRead_RejectDropsSidechainAndErrorEntriesButKeepsAbsentOrExplicitNull(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	entry := func(extra string) string {
		return fmt.Sprintf(
			`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"x"}]}%s,"timestamp":"2026-08-18T10:00:00Z"}`,
			extra)
	}
	writeLines(t, path, []string{
		entry(`,"isSidechain":true`),
		entry(`,"isApiErrorMessage":true`),
		entry(``),
		entry(`,"isSidechain":null`),
	})

	got, err := transcript.Read(claudeDescriptor(), path, 0)

	require.NoError(t, err)
	assert.Len(t, got.Messages, 2, "only the entries without a TRUE reject flag survive")
}

// TestRead_IncrementalReadsOnlyReturnWhatWasAppendedSinceTheLastOffset is the
// core promise a live session depends on: a read that started at the last
// Offset must never hand back a message already delivered.
func TestRead_IncrementalReadsOnlyReturnWhatWasAppendedSinceTheLastOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	writeLines(t, path, []string{claudeLine("first", "2026-08-18T10:00:00Z")})

	first, err := transcript.Read(claudeDescriptor(), path, 0)
	require.NoError(t, err)
	require.Len(t, first.Messages, 1)
	assert.Equal(t, "first", first.Messages[0].Text)

	appendLine(t, path, claudeLine("second", "2026-08-18T10:00:01Z"))

	second, err := transcript.Read(claudeDescriptor(), path, first.Offset)
	require.NoError(t, err)
	require.Len(t, second.Messages, 1, "must not replay what the first read already returned")
	assert.Equal(t, "second", second.Messages[0].Text)
	assert.Greater(t, second.Offset, first.Offset)
}

// TestRead_APartialFinalLineIsReturnedOnlyOnceItIsCompleted is the whole of this
// package's tolerance for a file that grows while it is open: a line still being
// written by the CLI must never be handed to the JSON decoder half-formed.
func TestRead_APartialFinalLineIsReturnedOnlyOnceItIsCompleted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	complete := claudeLine("done", "2026-08-18T10:00:00Z")
	full := claudeLine("still writing", "2026-08-18T10:00:01Z")
	splitAt := strings.Index(full, "writ") + len("writ")
	partial, rest := full[:splitAt], full[splitAt:]
	require.NoError(t, os.WriteFile(path, []byte(complete+"\n"+partial), 0o644))

	got, err := transcript.Read(claudeDescriptor(), path, 0)

	require.NoError(t, err)
	require.Len(t, got.Messages, 1, "the partial tail must not reach the decoder")
	assert.Equal(t, "done", got.Messages[0].Text)
	assert.Equal(t, int64(len(complete)+1), got.Offset, "stops before the partial line, not mid-way through it")

	appendLine(t, path, rest)

	second, err := transcript.Read(claudeDescriptor(), path, got.Offset)
	require.NoError(t, err)
	require.Len(t, second.Messages, 1)
	assert.Equal(t, "still writing", second.Messages[0].Text)
}

// TestRead_MalformedLinesAreSkippedWithoutError: a provider's file is not a
// contract, so one line of the wrong shape must cost only that line, never the
// lines around it and never the read itself.
func TestRead_MalformedLinesAreSkippedWithoutError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	good := claudeLine("kept", "2026-08-18T10:00:00Z")
	writeLines(t, path, []string{
		"this is not json at all",
		"[1,2]",
		"",
		`{"foo":"bar"}`,
		good,
	})

	got, err := transcript.Read(claudeDescriptor(), path, 0)

	require.NoError(t, err)
	require.Len(t, got.Messages, 1, "every bad line is skipped; only the well-formed one survives")
	assert.Equal(t, "kept", got.Messages[0].Text)
}

// TestRead_PureBinaryGarbageYieldsNoMessagesAndNoError covers a transcript that
// is not text at all — the daemon must never panic on a file it does not
// control, whatever a provider (or a corrupted disk) puts in it.
func TestRead_PureBinaryGarbageYieldsNoMessagesAndNoError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	garbage := make([]byte, 4096)
	for i := range garbage {
		garbage[i] = byte(i % 256)
	}
	require.NoError(t, os.WriteFile(path, garbage, 0o644))

	got, err := transcript.Read(claudeDescriptor(), path, 0)

	require.NoError(t, err)
	assert.Empty(t, got.Messages)
}

// TestRead_AMissingFileReturnsNoMessagesNoErrorAndAnUnchangedOffset: a session
// whose CLI has not written its transcript yet must read exactly like an empty
// one, not like a failure.
func TestRead_AMissingFileReturnsNoMessagesNoErrorAndAnUnchangedOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.jsonl")

	got, err := transcript.Read(claudeDescriptor(), path, 42)

	require.NoError(t, err)
	assert.Empty(t, got.Messages)
	assert.Equal(t, int64(42), got.Offset)
}

// TestEnd_ReturnsTheFilesCurrentSize pins where a freshly watched session
// starts: at the end of what is already on disk.
func TestEnd_ReturnsTheFilesCurrentSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	line := claudeLine("hi", "2026-08-18T10:00:00Z")
	writeLines(t, path, []string{line})

	got := transcript.End(claudeDescriptor(), path)

	assert.Equal(t, int64(len(line)+1), got)
}

// TestEnd_AMissingFileReturnsZero: the same answer as a file that has not been
// written to yet, because to a session that has not started they are the same
// fact.
func TestEnd_AMissingFileReturnsZero(t *testing.T) {
	dir := t.TempDir()

	got := transcript.End(claudeDescriptor(), filepath.Join(dir, "nope.jsonl"))

	assert.Equal(t, int64(0), got)
}

// TestRead_AFileShorterThanTheOffsetRestartsAtItsNewEndRatherThanReplayingIt
// covers a rotated or replaced session file at the same path: the old offset
// belongs to a different file's history, and replaying it as this turn's speech
// is a worse failure than missing a message.
func TestRead_AFileShorterThanTheOffsetRestartsAtItsNewEndRatherThanReplayingIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	writeLines(t, path, []string{claudeLine("this was a long earlier session", "2026-08-18T10:00:00Z")})
	priorOffset := transcript.End(claudeDescriptor(), path)

	short := claudeLine("new", "2026-08-18T11:00:00Z")
	writeLines(t, path, []string{short})
	require.Less(t, int64(len(short)+1), priorOffset, "the fixture must actually be shorter than the old offset")

	got, err := transcript.Read(claudeDescriptor(), path, priorOffset)

	require.NoError(t, err)
	assert.Empty(t, got.Messages, "the rotated file's content must never be replayed as this turn's speech")
	assert.Equal(t, int64(len(short)+1), got.Offset, "resumes at the new file's end, not at 0")
}

// TestRead_MaxMessageBytesTruncatesAnOverLongMessageAndMarksIt: a message past
// the ceiling is recorded truncated and MARKED, never silently clipped, because
// a reader must be able to see there was more.
func TestRead_MaxMessageBytesTruncatesAnOverLongMessageAndMarksIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	writeLines(t, path, []string{claudeLine(strings.Repeat("a", 100), "2026-08-18T10:00:00Z")})

	d := claudeDescriptor()
	d.Transcript.MaxMessageBytes = 10

	got, err := transcript.Read(d, path, 0)

	require.NoError(t, err)
	require.Len(t, got.Messages, 1)
	msg := got.Messages[0]
	assert.True(t, msg.Truncated)
	assert.True(t, strings.HasSuffix(msg.Text, "\n\n[crowbar: message truncated]"),
		"a truncated message must say so in the text itself, not only in the flag")
}

// TestRead_MaxReadBytesBoundsOneReadAndTheFollowUpGetsTheRest covers the ceiling
// on ONE read: the cut must land on a whole-line boundary inside the window, not
// at a raw byte count, and whatever it leaves behind must still be there on the
// next read.
func TestRead_MaxReadBytesBoundsOneReadAndTheFollowUpGetsTheRest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	line1 := claudeLine("one", "2026-08-18T10:00:00Z") + "\n"
	line2 := claudeLine("two", "2026-08-18T10:00:01Z") + "\n"
	line3 := claudeLine("three", "2026-08-18T10:00:02Z") + "\n"
	require.NoError(t, os.WriteFile(path, []byte(line1+line2+line3), 0o644))

	d := claudeDescriptor()
	d.Transcript.MaxReadBytes = len(line1) + len(line2)/2

	first, err := transcript.Read(d, path, 0)
	require.NoError(t, err)
	assert.True(t, first.Truncated, "the window was smaller than the file")
	require.Len(t, first.Messages, 1, "only the whole line inside the window is decoded")
	assert.Equal(t, "one", first.Messages[0].Text)
	assert.Equal(t, int64(len(line1)), first.Offset, "stops at the line boundary, not mid-line inside the window")

	d.Transcript.MaxReadBytes = 0
	second, err := transcript.Read(d, path, first.Offset)
	require.NoError(t, err)
	require.Len(t, second.Messages, 2, "the follow-up read returns everything the first one left behind")
	assert.Equal(t, "two", second.Messages[0].Text)
	assert.Equal(t, "three", second.Messages[1].Text)
	assert.False(t, second.Truncated)
}

// TestRead_ASingleLineLongerThanMaxReadBytesNeverWedgesTheOffset is the
// pathological case MaxReadBytes exists to survive: one entry bigger than a
// whole read is allowed to be. It can never decode to a message, but the offset
// must still advance every call, or a daemon polling this transcript spins
// forever on the same byte.
func TestRead_ASingleLineLongerThanMaxReadBytesNeverWedgesTheOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	line := claudeLine(strings.Repeat("a", 2000), "2026-08-18T10:00:00Z") + "\n"
	require.NoError(t, os.WriteFile(path, []byte(line), 0o644))

	d := claudeDescriptor()
	d.Transcript.MaxReadBytes = 100

	size := int64(len(line))
	offset := int64(0)
	for iterations := 0; offset < size; iterations++ {
		require.Less(t, iterations, 1000, "the offset must make forward progress every call, never wedge")
		got, err := transcript.Read(d, path, offset)
		require.NoError(t, err)
		assert.Empty(t, got.Messages, "a fragment of one oversized line can never decode to a message")
		require.Greater(t, got.Offset, offset, "each read must advance even without a line boundary in the window")
		offset = got.Offset
	}
	assert.Equal(t, size, offset)
}

// TestDeclared_FalseForAnEmptyDescriptorOrNil pins the degradation contract's
// gate: a provider naming no transcript block behaves exactly as it did before
// this package existed.
func TestDeclared_FalseForAnEmptyDescriptorOrNil(t *testing.T) {
	assert.False(t, transcript.Declared(&spec.Descriptor{}))
	assert.False(t, transcript.Declared(nil))
}

// TestDeclared_FalseForANonJSONLFormat: "jsonl" is the only format the engine
// reads, and both shipped providers write it — anything else is treated as no
// transcript at all rather than an attempt to parse an unknown shape.
func TestDeclared_FalseForANonJSONLFormat(t *testing.T) {
	d := claudeDescriptor()
	d.Transcript.Format = "ndjson"

	assert.False(t, transcript.Declared(d))
}

// TestRead_AnUndeclaredDescriptorReturnsNothingAndLeavesTheOffsetAlone covers the
// caller-facing half of the degradation contract: an undeclared provider's Read
// must be indistinguishable from one that was never called.
func TestRead_AnUndeclaredDescriptorReturnsNothingAndLeavesTheOffsetAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	writeLines(t, path, []string{claudeLine("hi", "2026-08-18T10:00:00Z")})

	got, err := transcript.Read(&spec.Descriptor{}, path, 7)

	require.NoError(t, err)
	assert.Empty(t, got.Messages)
	assert.Equal(t, int64(7), got.Offset, "an undeclared descriptor must not move the cursor")
}

// TestEnd_AnUndeclaredDescriptorReturnsZero is End's half of the same contract.
func TestEnd_AnUndeclaredDescriptorReturnsZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	writeLines(t, path, []string{claudeLine("hi", "2026-08-18T10:00:00Z")})

	assert.Equal(t, int64(0), transcript.End(&spec.Descriptor{}, path))
}

// TestRead_NeverWritesToTheTranscriptOrItsDirectory is the proof for the
// property this whole package exists to guarantee: the provider's file belongs
// to the provider, and Crowbar has no business writing there. Locking the file
// to 0444 and its directory to 0555 before calling Read and End, then comparing
// the file's bytes, size and mtime and the directory's own entries before and
// after, is what makes "read-only" a tested claim rather than a comment.
func TestRead_NeverWritesToTheTranscriptOrItsDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission semantics")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permissions, so no write would be blocked to prove anything")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	writeLines(t, path, []string{
		claudeLine("first", "2026-08-18T10:00:00Z"),
		claudeLine("second", "2026-08-18T10:00:01Z"),
	})

	before, err := os.ReadFile(path)
	require.NoError(t, err)
	infoBefore, err := os.Stat(path)
	require.NoError(t, err)
	entriesBefore, err := os.ReadDir(dir)
	require.NoError(t, err)

	require.NoError(t, os.Chmod(path, 0o444))
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o755)
		_ = os.Chmod(path, 0o644)
	})

	d := claudeDescriptor()
	got, err := transcript.Read(d, path, 0)
	require.NoError(t, err, "a read-only file and directory must not defeat a read")
	require.Len(t, got.Messages, 2)
	assert.Equal(t, "first", got.Messages[0].Text)
	assert.Equal(t, "second", got.Messages[1].Text)
	assert.Equal(t, infoBefore.Size(), transcript.End(d, path))

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after, "the bytes on disk must be identical")

	infoAfter, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, infoBefore.Size(), infoAfter.Size())
	assert.True(t, infoBefore.ModTime().Equal(infoAfter.ModTime()), "a read must never touch mtime")

	entriesAfter, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entriesAfter, len(entriesBefore), "no lock file, temp file or anything else may appear")
	for i, e := range entriesBefore {
		assert.Equal(t, e.Name(), entriesAfter[i].Name())
	}
}

// TestRead_CodexFixtureWorksThroughTheSameCodeWithOnlyADifferentDescriptor is
// what proves nothing provider-specific lives in Go: the same Read, driven by a
// descriptor whose paths and match keys differ at every level, produces the same
// kind of message.
func TestRead_CodexFixtureWorksThroughTheSameCodeWithOnlyADifferentDescriptor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	writeLines(t, path, []string{
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi from codex"}]},"timestamp":"2026-08-18T10:00:00Z"}`,
	})

	got, err := transcript.Read(codexDescriptor(), path, 0)

	require.NoError(t, err)
	require.Len(t, got.Messages, 1)
	assert.Equal(t, "hi from codex", got.Messages[0].Text)
}

// TestDeclared_ShippedClaudeAndCodexBothDeclareATranscript loads the REAL
// embedded descriptors rather than a hand-built fixture, so an edit that drops
// or malforms either provider's transcript block fails here instead of only
// silently un-fixing the feature in production.
func TestDeclared_ShippedClaudeAndCodexBothDeclareATranscript(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		t.Run(id, func(t *testing.T) {
			d, err := descriptor.Resolve(context.Background(), t.TempDir(), id)
			require.NoError(t, err)
			assert.True(t, transcript.Declared(d))
		})
	}
}

// TestRead_AnOversizedLineCostsOnlyItself is the regression for a silent loss
// this package's own promise forbids.
//
// The first implementation split lines with a bufio.Scanner, whose token buffer
// is bounded — and a Scanner that hits an oversized token stops FOR GOOD. Because
// the read has already computed its new offset from the whole window before the
// lines are decoded, every well-formed message after the oversized one was
// dropped AND skipped past: unreadable then, unreadable forever. A single fat
// line in a transcript could therefore swallow the rest of a turn.
func TestRead_AnOversizedLineCostsOnlyItself(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	fat := `{"type":"attachment","blob":"` + strings.Repeat("z", 2<<20) + `"}`
	lines := []string{
		claudeLine("before the fat line", "2026-08-18T10:00:00Z"),
		fat,
		claudeLine("after the fat line", "2026-08-18T10:00:01Z"),
	}
	writeLines(t, path, lines)

	got, err := transcript.Read(claudeDescriptor(), path, 0)
	require.NoError(t, err)
	require.Len(t, got.Messages, 2,
		"a line too big to decode must cost that line and nothing else")
	assert.Equal(t, "before the fat line", got.Messages[0].Text)
	assert.Equal(t, "after the fat line", got.Messages[1].Text)

	size, statErr := os.Stat(path)
	require.NoError(t, statErr)
	assert.Equal(t, size.Size(), got.Offset,
		"and the offset must still land at the end of the last complete line")
}
