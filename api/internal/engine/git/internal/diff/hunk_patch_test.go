package diff_test

import (
	"context"
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHunkPatch_Basic(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "hello.go", "package main\n\nfunc hello() {}\n")
	mustGit(t, dir, "add", "hello.go")
	mustGit(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "hello.go", "package main\n\nfunc hello() { return }\n")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, false)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.NotEmpty(t, files[0].Hunks)

	hunkID := files[0].Hunks[0].HunkID

	patch, err := diff.HunkPatch(ctx, dir, "hello.go", hunkID, false)
	require.NoError(t, err)
	assert.Contains(t, patch, "diff --git a/hello.go b/hello.go")
	assert.Contains(t, patch, "--- a/hello.go")
	assert.Contains(t, patch, "+++ b/hello.go")
	assert.Contains(t, patch, "@@")
}

func TestHunkPatch_StaleHunk_FileNotFound(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "hello.go", "package main\n\nfunc hello() {}\n")
	mustGit(t, dir, "add", "hello.go")
	mustGit(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "hello.go", "package main\n\nfunc hello() { return }\n")

	ctx := context.Background()
	_, err := diff.HunkPatch(ctx, dir, "nonexistent.go", "any-hunk-id", false)
	require.ErrorIs(t, err, diff.ErrStaleHunk)
}

func TestHunkPatch_StaleHunk_HunkIDNotFound(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "hello.go", "package main\n\nfunc hello() {}\n")
	mustGit(t, dir, "add", "hello.go")
	mustGit(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "hello.go", "package main\n\nfunc hello() { return }\n")

	ctx := context.Background()
	_, err := diff.HunkPatch(ctx, dir, "hello.go", "000000000000", false)
	require.ErrorIs(t, err, diff.ErrStaleHunk)
}

func TestHunkPatch_PatchContainsDiffLines(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "main.go", "package main\n\nfunc foo() {}\nfunc bar() {}\n")
	mustGit(t, dir, "add", "main.go")
	mustGit(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "main.go", "package main\n\nfunc foo() { return }\nfunc bar() {}\n")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, false)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.NotEmpty(t, files[0].Hunks)

	hunkID := files[0].Hunks[0].HunkID

	patch, err := diff.HunkPatch(ctx, dir, "main.go", hunkID, false)
	require.NoError(t, err)

	lines := strings.Split(patch, "\n")
	var hasDiffLine bool
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			hasDiffLine = true
			break
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			hasDiffLine = true
			break
		}
	}
	assert.True(t, hasDiffLine, "patch must contain at least one +/- line")
}

func TestHunkPatch_CleanRepo_ReturnsStaleHunk(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "hello.go", "package main\n\nfunc hello() {}\n")
	mustGit(t, dir, "add", "hello.go")
	mustGit(t, dir, "commit", "-m", "initial")

	ctx := context.Background()
	_, err := diff.HunkPatch(ctx, dir, "hello.go", "aabbccddeeff", false)
	require.ErrorIs(t, err, diff.ErrStaleHunk)
}

func TestHunkPatch_InvalidRepo_ReturnsError(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir() // not a git repo
	_, err := diff.HunkPatch(ctx, dir, "any.go", "aabbccddeeff", false)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, diff.ErrStaleHunk)
}
