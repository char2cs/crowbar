// Package diff (internal tests) exercises patch.go's private helpers directly:
// patchError's branch table and cutHunk's malformed/edge inputs are pure-function
// cases a real git subprocess essentially never produces, and patchCopier's
// error latch needs a writer that fails on demand.
package diff

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatchError_CopyErrTakesPriorityOverEverythingElse(t *testing.T) {
	copyErr := errors.New("copy failed")

	got := patchError(true, copyErr, errors.New("close failed"), errors.New("wait failed"))

	assert.Same(t, copyErr, got)
}

func TestPatchError_TruncatedSuppressesCloseAndWaitErrors(t *testing.T) {
	got := patchError(true, nil, errors.New("close failed from killing git early"), errors.New("wait failed"))

	assert.NoError(t, got)
}

func TestPatchError_UntruncatedCloseErrReported(t *testing.T) {
	closeErr := errors.New("close failed")

	got := patchError(false, nil, closeErr, nil)

	assert.Same(t, closeErr, got)
}

func TestPatchError_FallsBackToWaitErr(t *testing.T) {
	waitErr := errors.New("git exited non-zero")

	got := patchError(false, nil, nil, waitErr)

	assert.Same(t, waitErr, got)
}

func TestCutHunk_RoomTooSmallReturnsNil(t *testing.T) {
	hunk := []string{"@@ -1,3 +1,3 @@\n", " a\n", "-b\n", "+B\n"}

	got := cutHunk(hunk, 1)

	assert.Nil(t, got)
}

func TestCutHunk_EmptyHunkReturnsNil(t *testing.T) {
	got := cutHunk(nil, 5)
	assert.Nil(t, got)
}

func TestCutHunk_AlreadyFitsReturnsHunkUnchanged(t *testing.T) {
	hunk := []string{"@@ -1,1 +1,1 @@\n", "-a\n", "+A\n"}

	got := cutHunk(hunk, 10)

	assert.Equal(t, hunk, got)
}

func TestCutHunk_UnparseableHeaderReturnsNil(t *testing.T) {
	hunk := []string{"not a hunk header\n", " a\n", "-b\n", "+B\n", " c\n"}

	got := cutHunk(hunk, 3)

	assert.Nil(t, got)
}

// TestCutHunk_AllContextNoChangesReturnsNil covers the guard against emitting a
// hunk whose surviving body changes nothing — git itself rejects such a patch
// as corrupt, so cutting must drop it rather than send a header over dead
// context lines.
func TestCutHunk_AllContextNoChangesReturnsNil(t *testing.T) {
	hunk := []string{"@@ -1,5 +1,5 @@\n", " a\n", " b\n", " c\n", " d\n", " e\n"}

	got := cutHunk(hunk, 3)

	assert.Nil(t, got)
}

// TestCutHunk_RewritesHeaderCountsForSurvivingBody is the normal cutting case:
// the kept lines' own +/- counts replace the original header's promise. room=3
// keeps only the header plus the first 2 of the hunk's 3 body lines (room-1),
// dropping the trailing context line — the header must then promise 1 old / 1
// new line, not the original 3.
func TestCutHunk_RewritesHeaderCountsForSurvivingBody(t *testing.T) {
	hunk := []string{"@@ -1,3 +1,3 @@\n", "-a\n", "+A\n", " c\n"}

	got := cutHunk(hunk, 3)

	require.Len(t, got, 3)
	assert.Equal(t, "@@ -1,1 +1,1 @@\n", got[0])
	assert.Equal(t, []string{"-a\n", "+A\n"}, got[1:])
}

// TestCutHunk_NoNewlineMarkerCountsNeitherSide pins the "\ No newline at end of
// file" marker's accounting in the recount loop: it annotates the preceding
// line rather than occupying a line of its own, so it must contribute to
// neither oldCount nor newCount (and not to changes) while the real +/- lines
// around it still do.
func TestCutHunk_NoNewlineMarkerCountsNeitherSide(t *testing.T) {
	hunk := []string{
		"@@ -1,3 +1,3 @@\n",
		"-old\n",
		"\\ No newline at end of file\n",
		"+new\n",
		" ctx\n",
	}

	got := cutHunk(hunk, 4)

	require.Len(t, got, 4)
	assert.Equal(t, "@@ -1,1 +1,1 @@\n", got[0], "the no-newline marker must not inflate either side's recounted total")
	assert.Equal(t, hunk[1:4], got[1:])
}

type failingWriter struct {
	calls int
	err   error
}

func (w *failingWriter) Write(p []byte) (int, error) {
	w.calls++
	return 0, w.err
}

func TestPatchCopier_WriteLatchesErrAndStopsWriting(t *testing.T) {
	fw := &failingWriter{err: errors.New("disk full")}
	c := &patchCopier{out: fw}

	c.write("first\n")
	require.Error(t, c.err)
	assert.Equal(t, 1, fw.calls)

	c.write("second\n")
	assert.Equal(t, 1, fw.calls, "write must no-op once c.err is set, not attempt another write")
}

func TestPatchCopier_ObserveNonEOFErrorLatchesErr(t *testing.T) {
	c := &patchCopier{}
	readErr := errors.New("broken pipe")

	c.observe(readErr)

	assert.Same(t, readErr, c.err)
	assert.False(t, c.done)
}
