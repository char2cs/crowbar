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

func TestCutHunk_AllContextNoChangesReturnsNil(t *testing.T) {
	hunk := []string{"@@ -1,5 +1,5 @@\n", " a\n", " b\n", " c\n", " d\n", " e\n"}

	got := cutHunk(hunk, 3)

	assert.Nil(t, got)
}

func TestCutHunk_RewritesHeaderCountsForSurvivingBody(t *testing.T) {
	hunk := []string{"@@ -1,3 +1,3 @@\n", "-a\n", "+A\n", " c\n"}

	got := cutHunk(hunk, 3)

	require.Len(t, got, 3)
	assert.Equal(t, "@@ -1,1 +1,1 @@\n", got[0])
	assert.Equal(t, []string{"-a\n", "+A\n"}, got[1:])
}

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
