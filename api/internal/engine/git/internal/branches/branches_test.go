package branches_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/branches"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

func initRepo(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()
	ctx := context.Background()
	r := exec.Git(ctx, dir, "init", "-b", "main")
	require.Equal(t, 0, r.ExitCode, r.Stderr)
	_ = exec.Git(ctx, dir, "config", "user.email", "test@test.com")
	_ = exec.Git(ctx, dir, "config", "user.name", "Test")
	return dir
}

func makeCommit(
	t *testing.T,
	dir string,
	filename string,
	content string,
	message string,
) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	_ = exec.Git(ctx, dir, "add", filename)
	r := exec.Git(ctx, dir, "commit", "-m", message)
	require.Equal(t, 0, r.ExitCode, r.Stderr)
}

func findBranch(
	bs []domain.Branch,
	name string,
) *domain.Branch {
	for i := range bs {
		if bs[i].Name == name {
			return &bs[i]
		}
	}
	return nil
}

func currentBranch(
	bs []domain.Branch,
) string {
	for _, b := range bs {
		if b.IsCurrent {
			return b.Name
		}
	}
	return ""
}

func TestList_SingleBranch(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	bs, err := branches.List(ctx, dir)

	require.NoError(t, err)
	require.Len(t, bs, 1)
	assert.Equal(t, "main", bs[0].Name)
	assert.True(t, bs[0].IsCurrent)
	assert.False(t, bs[0].IsRemote)
}

func TestList_MultipleBranches(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	err := branches.Create(ctx, dir, "feature", "", false)
	require.NoError(t, err)

	bs, err := branches.List(ctx, dir)

	require.NoError(t, err)
	require.Len(t, bs, 2)
	assert.NotNil(t, findBranch(bs, "main"))
	assert.NotNil(t, findBranch(bs, "feature"))
}

func TestList_CurrentBranch(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	err := branches.Create(ctx, dir, "other", "", true)
	require.NoError(t, err)

	bs, err := branches.List(ctx, dir)

	require.NoError(t, err)
	assert.Equal(t, "other", currentBranch(bs))
}

func TestList_RemoteBranch(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	remoteDir := t.TempDir()
	r := exec.Git(ctx, remoteDir, "init", "--bare", "-b", "main")
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	r = exec.Git(ctx, dir, "remote", "add", "origin", remoteDir)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	r = exec.Git(ctx, dir, "push", "-u", "origin", "main")
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	bs, err := branches.List(ctx, dir)

	require.NoError(t, err)

	var remoteNames []string
	for _, b := range bs {
		if b.IsRemote {
			remoteNames = append(remoteNames, b.Name)
		}
	}
	require.NotEmpty(t, remoteNames)
	assert.Contains(t, remoteNames, "origin/main")
}

func TestList_AheadBehind(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	remoteDir := t.TempDir()
	r := exec.Git(ctx, remoteDir, "init", "--bare", "-b", "main")
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	r = exec.Git(ctx, dir, "remote", "add", "origin", remoteDir)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	r = exec.Git(ctx, dir, "push", "-u", "origin", "main")
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	makeCommit(t, dir, "extra.txt", "extra\n", "local commit")

	bs, err := branches.List(ctx, dir)

	require.NoError(t, err)

	b := findBranch(bs, "main")
	require.NotNil(t, b)
	require.NotNil(t, b.Ahead)
	assert.Equal(t, 1, *b.Ahead)
}

func TestCreate_NoSwitch(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	err := branches.Create(ctx, dir, "new-branch", "", false)

	require.NoError(t, err)

	bs, err := branches.List(ctx, dir)
	require.NoError(t, err)

	assert.NotNil(t, findBranch(bs, "new-branch"))
	assert.Equal(t, "main", currentBranch(bs))
}

func TestCreate_WithSwitch(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	err := branches.Create(ctx, dir, "feature-x", "", true)

	require.NoError(t, err)

	bs, err := branches.List(ctx, dir)
	require.NoError(t, err)
	assert.Equal(t, "feature-x", currentBranch(bs))
}

func TestCreate_WithSource(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	err := branches.Create(ctx, dir, "from-main", "main", false)

	require.NoError(t, err)

	bs, err := branches.List(ctx, dir)
	require.NoError(t, err)
	assert.NotNil(t, findBranch(bs, "from-main"))
}

func TestRename(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	err := branches.Create(ctx, dir, "old-name", "", false)
	require.NoError(t, err)

	err = branches.Rename(ctx, dir, "old-name", "new-name")

	require.NoError(t, err)

	bs, err := branches.List(ctx, dir)
	require.NoError(t, err)

	assert.Nil(t, findBranch(bs, "old-name"))
	assert.NotNil(t, findBranch(bs, "new-name"))
}

func TestDelete(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	err := branches.Create(ctx, dir, "to-delete", "", false)
	require.NoError(t, err)

	err = branches.Delete(ctx, dir, "to-delete")

	require.NoError(t, err)

	bs, err := branches.List(ctx, dir)
	require.NoError(t, err)
	assert.Nil(t, findBranch(bs, "to-delete"))
}

func TestForceDelete(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	err := branches.Create(ctx, dir, "unmerged", "", true)
	require.NoError(t, err)
	makeCommit(t, dir, "extra.txt", "extra\n", "unmerged commit")

	r := exec.Git(ctx, dir, "checkout", "main")
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	err = branches.ForceDelete(ctx, dir, "unmerged")

	require.NoError(t, err)

	bs, err := branches.List(ctx, dir)
	require.NoError(t, err)
	assert.Nil(t, findBranch(bs, "unmerged"))
}

func TestSwitch(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	err := branches.Create(ctx, dir, "target", "", false)
	require.NoError(t, err)

	err = branches.Switch(ctx, dir, "target")

	require.NoError(t, err)

	bs, err := branches.List(ctx, dir)
	require.NoError(t, err)
	assert.Equal(t, "target", currentBranch(bs))
}

func TestList_LastCommitDate(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	bs, err := branches.List(ctx, dir)

	require.NoError(t, err)
	require.Len(t, bs, 1)
	assert.NotNil(t, bs[0].LastCommitDate)
}

// --- Error path tests ---

func TestList_InvalidRepo(
	t *testing.T,
) {
	ctx := context.Background()

	_, err := branches.List(ctx, "/nonexistent/path/that/does/not/exist")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branches: list")
}

func TestCreate_AlreadyExists_NoSwitch(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	err := branches.Create(ctx, dir, "dupe", "", false)
	require.NoError(t, err)

	err = branches.Create(ctx, dir, "dupe", "", false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branches: create")
}

func TestCreate_AlreadyExists_WithSwitch(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	// "main" already exists — checkout -b main should fail.
	err := branches.Create(ctx, dir, "main", "", true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branches: create")
}

func TestRename_NonExistentBranch(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	err := branches.Rename(ctx, dir, "nonexistent", "new-name")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branches: rename")
}

func TestDelete_NonExistentBranch(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	err := branches.Delete(ctx, dir, "nonexistent")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branches: delete")
}

func TestForceDelete_NonExistentBranch(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	err := branches.ForceDelete(ctx, dir, "nonexistent")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branches: force-delete")
}

func TestSwitch_NonExistentBranch(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	err := branches.Switch(ctx, dir, "nonexistent")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branches: switch")
}

// --- parseTrack / parseTrackSegment: ahead+behind, behind-only paths ---

func TestList_AheadAndBehind(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "base.txt", "base\n", "base commit")

	remoteDir := t.TempDir()
	r := exec.Git(ctx, remoteDir, "init", "--bare", "-b", "main")
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	r = exec.Git(ctx, dir, "remote", "add", "origin", remoteDir)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	r = exec.Git(ctx, dir, "push", "-u", "origin", "main")
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	// Push a commit to remote via a clone so the remote is ahead of local.
	cloneDir := t.TempDir()
	r = exec.Git(ctx, cloneDir, "clone", remoteDir, ".")
	require.Equal(t, 0, r.ExitCode, r.Stderr)
	_ = exec.Git(ctx, cloneDir, "config", "user.email", "test@test.com")
	_ = exec.Git(ctx, cloneDir, "config", "user.name", "Test")

	clonePath := filepath.Join(cloneDir, "remote.txt")
	require.NoError(t, os.WriteFile(clonePath, []byte("remote\n"), 0600))
	_ = exec.Git(ctx, cloneDir, "add", "remote.txt")
	r = exec.Git(ctx, cloneDir, "commit", "-m", "remote commit")
	require.Equal(t, 0, r.ExitCode, r.Stderr)
	r = exec.Git(ctx, cloneDir, "push", "origin", "main")
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	// Local adds its own commit (ahead) and fetches so it knows it is also behind.
	makeCommit(t, dir, "local.txt", "local\n", "local commit")
	_ = exec.Git(ctx, dir, "fetch", "origin")

	bs, err := branches.List(ctx, dir)
	require.NoError(t, err)

	b := findBranch(bs, "main")
	require.NotNil(t, b)
	require.NotNil(t, b.Ahead, "expected Ahead to be set")
	require.NotNil(t, b.Behind, "expected Behind to be set")
	assert.Equal(t, 1, *b.Ahead)
	assert.Equal(t, 1, *b.Behind)
}

func TestList_BehindOnly(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "base.txt", "base\n", "base commit")

	remoteDir := t.TempDir()
	r := exec.Git(ctx, remoteDir, "init", "--bare", "-b", "main")
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	r = exec.Git(ctx, dir, "remote", "add", "origin", remoteDir)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	r = exec.Git(ctx, dir, "push", "-u", "origin", "main")
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	// Push a commit to remote via a clone.
	cloneDir := t.TempDir()
	r = exec.Git(ctx, cloneDir, "clone", remoteDir, ".")
	require.Equal(t, 0, r.ExitCode, r.Stderr)
	_ = exec.Git(ctx, cloneDir, "config", "user.email", "test@test.com")
	_ = exec.Git(ctx, cloneDir, "config", "user.name", "Test")

	clonePath := filepath.Join(cloneDir, "remote.txt")
	require.NoError(t, os.WriteFile(clonePath, []byte("remote\n"), 0600))
	_ = exec.Git(ctx, cloneDir, "add", "remote.txt")
	r = exec.Git(ctx, cloneDir, "commit", "-m", "remote commit")
	require.Equal(t, 0, r.ExitCode, r.Stderr)
	r = exec.Git(ctx, cloneDir, "push", "origin", "main")
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	// Fetch only — local has no new commits, so it is purely behind.
	_ = exec.Git(ctx, dir, "fetch", "origin")

	bs, err := branches.List(ctx, dir)
	require.NoError(t, err)

	b := findBranch(bs, "main")
	require.NotNil(t, b)
	require.NotNil(t, b.Behind, "expected Behind to be set")
	assert.Equal(t, 1, *b.Behind)
	assert.Nil(t, b.Ahead)
}

// --- White-box tests using exported internal helpers ---

func TestCreate_WithSwitchAndSource(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()
	makeCommit(t, dir, "file.txt", "hello\n", "initial")

	err := branches.Create(ctx, dir, "from-main-switched", "main", true)

	require.NoError(t, err)

	bs, err := branches.List(ctx, dir)
	require.NoError(t, err)
	assert.Equal(t, "from-main-switched", currentBranch(bs))
}

func TestParseRecord_TooFewLines(
	t *testing.T,
) {
	// A record with fewer than 5 lines should return ok=false.
	_, ok := branches.ExportedParseRecord("name\n*\n")
	assert.False(t, ok)
}

func TestParseRecord_EmptyRefname(
	t *testing.T,
) {
	// A record where the first line (refname) is blank should return ok=false.
	_, ok := branches.ExportedParseRecord("\n*\ntrack\n2024-01-01T00:00:00Z\nrefs/heads/main")
	assert.False(t, ok)
}

func TestParseList_WithBadRecord(
	t *testing.T,
) {
	// parseList skips records that parseRecord rejects.
	// Inject a bad record between two record separators.
	output := "bad\n---RECORD---\n"
	bs := branches.ExportedParseList(output)
	// "bad\n" is a single field record — fewer than 5 lines → skipped.
	assert.Empty(t, bs)
}

// --- Tests that inject a failing git runner to cover exec error paths ---

func TestList_ExecError(
	t *testing.T,
) {
	branches.SetErrorRunner(t.Cleanup)
	ctx := context.Background()

	_, err := branches.List(ctx, t.TempDir())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branches: list")
}

func TestCreate_ExecError_NoSwitch(
	t *testing.T,
) {
	branches.SetErrorRunner(t.Cleanup)
	ctx := context.Background()

	err := branches.Create(ctx, t.TempDir(), "x", "", false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branches: create")
}

func TestCreate_ExecError_WithSwitch(
	t *testing.T,
) {
	branches.SetErrorRunner(t.Cleanup)
	ctx := context.Background()

	err := branches.Create(ctx, t.TempDir(), "x", "", true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branches: create")
}

func TestRename_ExecError(
	t *testing.T,
) {
	branches.SetErrorRunner(t.Cleanup)
	ctx := context.Background()

	err := branches.Rename(ctx, t.TempDir(), "old", "new")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branches: rename")
}

func TestDelete_ExecError(
	t *testing.T,
) {
	branches.SetErrorRunner(t.Cleanup)
	ctx := context.Background()

	err := branches.Delete(ctx, t.TempDir(), "x")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branches: delete")
}

func TestForceDelete_ExecError(
	t *testing.T,
) {
	branches.SetErrorRunner(t.Cleanup)
	ctx := context.Background()

	err := branches.ForceDelete(ctx, t.TempDir(), "x")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branches: force-delete")
}

func TestSwitch_ExecError(
	t *testing.T,
) {
	branches.SetErrorRunner(t.Cleanup)
	ctx := context.Background()

	err := branches.Switch(ctx, t.TempDir(), "x")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branches: switch")
}
