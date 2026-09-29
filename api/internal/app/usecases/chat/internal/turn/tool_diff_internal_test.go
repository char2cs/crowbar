package turn

import (
	"testing"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/stretchr/testify/assert"
)

func TestEditDiff_ReplacementKeepsSharedLinesAsContext(t *testing.T) {
	got := editDiff("a/b.go", "one\ntwo\nthree\n", "one\n2\nthree\n")
	assert.Equal(t, "--- a/a/b.go\n+++ b/a/b.go\n@@ -1,3 +1,3 @@\n one\n-two\n+2\n three\n", got)
}

func TestEditDiff_NewContentIsAllAdditions(t *testing.T) {
	got := editDiff("n.txt", "", "hello\nworld")
	assert.Equal(t, "--- a/n.txt\n+++ b/n.txt\n@@ -0,0 +1,2 @@\n+hello\n+world\n", got)
}

func TestEditDiff_NothingToShowIsEmpty(t *testing.T) {
	assert.Empty(t, editDiff("", "a", "b"))
	assert.Empty(t, editDiff("f", "same", "same"))
}

func TestEditDiff_AbsolutePathHeadersAgreeOnOneFileName(t *testing.T) {
	got := editDiff("/repo/README.md", "a\n", "b\n")
	assert.Equal(t, "--- a/repo/README.md\n+++ b/repo/README.md\n@@ -1,1 +1,1 @@\n-a\n+b\n", got)
}

func TestPatchDiff_KeepsTheProvidersTrueLineNumbers(t *testing.T) {
	got := patchDiff("/repo/a.go", []engineagents.PatchHunk{
		{OldStart: 8, NewStart: 8, Lines: []string{" ctx", "-old", "+new", " tail"}},
		{OldStart: 40, NewStart: 40, Lines: []string{"-gone"}},
	})
	assert.Equal(t, "--- a/repo/a.go\n+++ b/repo/a.go\n"+
		"@@ -8,3 +8,3 @@\n ctx\n-old\n+new\n tail\n"+
		"@@ -40,1 +39,0 @@\n-gone\n", got)
}

func TestPatchDiff_NoHunksIsEmpty(t *testing.T) {
	assert.Empty(t, patchDiff("a.go", nil))
}
