package diff_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/diff"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initRepo(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "user.email", "test@example.com")
	mustGit(t, dir, "config", "user.name", "Test User")
	return dir
}

func currentBranch(
	t *testing.T,
	dir string,
) string {
	t.Helper()
	r := exec.Git(context.Background(), dir, "rev-parse", "--abbrev-ref", "HEAD")
	require.Equal(t, 0, r.ExitCode, "rev-parse HEAD failed: %s", r.Stderr)
	return strings.TrimSpace(r.Stdout)
}

func mustGit(
	t *testing.T,
	dir string,
	args ...string,
) {
	t.Helper()
	r := exec.Git(context.Background(), dir, args...)
	require.Equal(t, 0, r.ExitCode, "git %v failed: %s", args, r.Stderr)
}

func writeFile(
	t *testing.T,
	dir string,
	name string,
	content string,
) {
	t.Helper()
	path := filepath.Join(dir, name)
	err := os.MkdirAll(filepath.Dir(path), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(path, []byte(content), 0o644)
	require.NoError(t, err)
}

func headSHA(
	t *testing.T,
	dir string,
) string {
	t.Helper()
	r := exec.Git(context.Background(), dir, "rev-parse", "HEAD")
	return strings.TrimSpace(r.Stdout)
}

func collectHunkIDs(
	f gitdomain.FileDiff,
) map[string]bool {
	ids := make(map[string]bool)
	for _, h := range f.Hunks {
		ids[h.HunkID] = true
	}
	return ids
}

func TestWorkingTree_Unstaged(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "hello.go", "package main\n\nfunc hello() {}\n")
	mustGit(t, dir, "add", "hello.go")
	mustGit(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "hello.go", "package main\n\nfunc hello() { return }\n")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, false)
	require.NoError(t, err)
	require.Len(t, files, 1)

	f := files[0]
	assert.Equal(t, "hello.go", f.FilePath)
	assert.False(t, f.IsNew)
	assert.False(t, f.IsDeleted)
	assert.Greater(t, f.Additions+f.Deletions, 0)
	assert.NotEmpty(t, f.Hunks)
	assert.NotEmpty(t, f.Lines)
}

func TestWorkingTree_Staged(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "hello.go", "package main\n\nfunc hello() {}\n")
	mustGit(t, dir, "add", "hello.go")
	mustGit(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "hello.go", "package main\n\nfunc hello() { return }\n")
	mustGit(t, dir, "add", "hello.go")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)
	require.Len(t, files, 1)

	f := files[0]
	assert.Equal(t, "hello.go", f.FilePath)
	assert.False(t, f.IsNew)
	assert.False(t, f.IsDeleted)
	assert.Greater(t, f.Additions+f.Deletions, 0)
	assert.NotEmpty(t, f.Hunks)
}

func TestWorkingTree_NewFile(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "initial.go", "package main\n")
	mustGit(t, dir, "add", "initial.go")
	mustGit(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "new.go", "package main\n\nfunc newFunc() {}\n")
	mustGit(t, dir, "add", "new.go")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)
	require.Len(t, files, 1)

	f := files[0]
	assert.Equal(t, "new.go", f.FilePath)
	assert.True(t, f.IsNew)
	assert.False(t, f.IsDeleted)
}

func TestWorkingTree_DeletedFile(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "gone.go", "package main\n")
	mustGit(t, dir, "add", "gone.go")
	mustGit(t, dir, "commit", "-m", "initial")

	err := os.Remove(filepath.Join(dir, "gone.go"))
	require.NoError(t, err)
	mustGit(t, dir, "add", "gone.go")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)
	require.Len(t, files, 1)

	f := files[0]
	assert.Equal(t, "gone.go", f.FilePath)
	assert.True(t, f.IsDeleted)
	assert.False(t, f.IsNew)
}

func TestWorkingTree_RenamedFile(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "old_name.go", "package main\n\nfunc foo() {}\nfunc bar() {}\nfunc baz() {}\n")
	mustGit(t, dir, "add", "old_name.go")
	mustGit(t, dir, "commit", "-m", "initial")

	err := os.Rename(filepath.Join(dir, "old_name.go"), filepath.Join(dir, "new_name.go"))
	require.NoError(t, err)
	mustGit(t, dir, "add", "old_name.go", "new_name.go")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)
	require.NotEmpty(t, files)

	var renamed *gitdomain.FileDiff
	for i := range files {
		if files[i].IsRenamed {
			renamed = &files[i]
			break
		}
	}
	require.NotNil(t, renamed, "expected a renamed file")
	assert.Equal(t, "new_name.go", renamed.FilePath)
	assert.Equal(t, "old_name.go", renamed.OldPath)
}

func TestWorkingTree_BinaryFile(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "doc.bin", "initial placeholder\n")
	mustGit(t, dir, "add", "doc.bin")
	mustGit(t, dir, "commit", "-m", "initial")

	binaryContent := []byte{0x00, 0x01, 0x02, 0x03, 0xFF, 0xFE}
	err := os.WriteFile(filepath.Join(dir, "doc.bin"), binaryContent, 0o644)
	require.NoError(t, err)
	mustGit(t, dir, "add", "doc.bin")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)
	require.NotEmpty(t, files)

	var binFile *gitdomain.FileDiff
	for i := range files {
		if files[i].FilePath == "doc.bin" {
			binFile = &files[i]
			break
		}
	}
	require.NotNil(t, binFile)
	assert.True(t, binFile.IsBinary)
}

func TestCommit_RootCommit(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	mustGit(t, dir, "add", "main.go")
	mustGit(t, dir, "commit", "-m", "root commit")

	sha := headSHA(t, dir)

	ctx := context.Background()
	result, err := diff.Commit(ctx, dir, sha)
	require.NoError(t, err)

	assert.Equal(t, sha, result.CommitHash)
	assert.Equal(t, "root commit", result.CommitMessage)
	assert.NotEmpty(t, result.CommitAuthor)
	assert.NotNil(t, result.CommitDate)
	assert.NotEmpty(t, result.Files)
	assert.Greater(t, result.TotalAdditions, 0)
}

func TestCommit_NonRootCommit(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	mustGit(t, dir, "add", "main.go")
	mustGit(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "main.go", "package main\n\nfunc main() { println(\"hello\") }\n")
	mustGit(t, dir, "add", "main.go")
	mustGit(t, dir, "commit", "-m", "update main")

	sha := headSHA(t, dir)

	ctx := context.Background()
	result, err := diff.Commit(ctx, dir, sha)
	require.NoError(t, err)

	assert.Equal(t, sha, result.CommitHash)
	assert.Equal(t, "update main", result.CommitMessage)
	assert.NotEmpty(t, result.Files)
	assert.Equal(t, 1, result.TotalFiles)
	assert.Greater(t, result.TotalAdditions, 0)
	assert.Greater(t, result.TotalDeletions, 0)
}

func TestWorkingTree_HunkIDStability(t *testing.T) {
	dir := initRepo(t)

	padding := strings.Repeat("\n", 10)
	original := "package main" + padding +
		"func funcA() {\n\treturn\n}" + padding +
		"func funcB() {\n\treturn\n}\n"
	writeFile(t, dir, "main.go", original)
	mustGit(t, dir, "add", "main.go")
	mustGit(t, dir, "commit", "-m", "initial")

	ctx := context.Background()

	bothModified := "package main" + padding +
		"func funcA() {\n\t// modified A\n\treturn\n}" + padding +
		"func funcB() {\n\t// modified B\n\treturn\n}\n"
	writeFile(t, dir, "main.go", bothModified)
	mustGit(t, dir, "add", "main.go")

	filesAll, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)
	require.Len(t, filesAll, 1)
	require.GreaterOrEqual(t, len(filesAll[0].Hunks), 2, "expected at least 2 hunks for well-separated changes")
	allHunkIDs := collectHunkIDs(filesAll[0])

	mustGit(t, dir, "restore", "--staged", "main.go")
	mustGit(t, dir, "restore", "main.go")

	onlyAModified := "package main" + padding +
		"func funcA() {\n\t// modified A\n\treturn\n}" + padding +
		"func funcB() {\n\treturn\n}\n"
	writeFile(t, dir, "main.go", onlyAModified)
	mustGit(t, dir, "add", "main.go")

	filesPartial, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)
	require.Len(t, filesPartial, 1)
	partialHunkIDs := collectHunkIDs(filesPartial[0])

	for id := range partialHunkIDs {
		assert.Contains(t, allHunkIDs, id,
			"hunk ID from partial stage should match the same hunk in full stage")
	}
}

func TestWorkingTree_LineNumbers(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "nums.go", "package main\n\nfunc a() {}\nfunc b() {}\nfunc c() {}\n")
	mustGit(t, dir, "add", "nums.go")
	mustGit(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "nums.go", "package main\n\nfunc a() { return }\nfunc b() {}\nfunc c() {}\n")
	mustGit(t, dir, "add", "nums.go")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)
	require.Len(t, files, 1)

	for _, line := range files[0].Lines {
		switch line.LineType {
		case gitdomain.DiffLineAdded:
			require.NotNil(t, line.NewLineNumber)
		case gitdomain.DiffLineRemoved:
			require.NotNil(t, line.OldLineNumber)
		case gitdomain.DiffLineContext:
			require.NotNil(t, line.OldLineNumber)
			require.NotNil(t, line.NewLineNumber)
		}
	}
}

func TestWorkingTree_HunksMatchLines(t *testing.T) {
	dir := initRepo(t)

	writeFile(t, dir, "hunks.go", "package main\n\nfunc foo() {}\n")
	mustGit(t, dir, "add", "hunks.go")
	mustGit(t, dir, "commit", "-m", "initial")

	writeFile(t, dir, "hunks.go", "package main\n\nfunc foo() { return }\n")
	mustGit(t, dir, "add", "hunks.go")

	ctx := context.Background()
	files, err := diff.WorkingTree(ctx, dir, true)
	require.NoError(t, err)
	require.Len(t, files, 1)

	f := files[0]
	for _, hunk := range f.Hunks {
		assert.GreaterOrEqual(t, hunk.StartLine, 0)
		assert.GreaterOrEqual(t, hunk.EndLine, hunk.StartLine)
		assert.Less(t, hunk.EndLine, len(f.Lines))
		assert.NotEmpty(t, hunk.HunkID)
		assert.Len(t, hunk.HunkID, 12)
	}
}
