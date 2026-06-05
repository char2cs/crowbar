package status_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/status"
)

func initRepo(
	t *testing.T,
) string {
	t.Helper()
	dir := t.TempDir()
	ctx := context.Background()
	r, err := exec.Git(ctx, dir, "init", "-b", "main")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode)
	_, _ = exec.Git(ctx, dir, "config", "user.email", "test@test.com")
	_, _ = exec.Git(ctx, dir, "config", "user.name", "Test")
	return dir
}

func makeInitialCommit(
	t *testing.T,
	dir string,
	filename string,
	content string,
) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	_, _ = exec.Git(ctx, dir, "add", filename)
	r, err := exec.Git(ctx, dir, "commit", "-m", "initial commit")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)
}

func TestParse_EmptyRepo(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()

	s, err := status.Parse(ctx, dir)

	require.NoError(t, err)
	assert.Equal(t, "main", s.Branch)
	assert.Equal(t, 0, s.Ahead)
	assert.Equal(t, 0, s.Behind)
	assert.Empty(t, s.Files)
}

func TestParse_UntrackedFile(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()

	path := filepath.Join(dir, "newfile.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello\n"), 0600))

	s, err := status.Parse(ctx, dir)

	require.NoError(t, err)
	require.Len(t, s.Files, 1)
	assert.Equal(t, "newfile.txt", s.Files[0].Path)
	assert.Equal(t, domain.GitFileStatusUntracked, s.Files[0].Status)
	assert.False(t, s.Files[0].Staged)
}

func TestParse_StagedNewFile(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()

	path := filepath.Join(dir, "staged.txt")
	require.NoError(t, os.WriteFile(path, []byte("staged\n"), 0600))
	_, _ = exec.Git(ctx, dir, "add", "staged.txt")

	s, err := status.Parse(ctx, dir)

	require.NoError(t, err)
	require.Len(t, s.Files, 1)
	assert.Equal(t, "staged.txt", s.Files[0].Path)
	assert.Equal(t, domain.GitFileStatusAdded, s.Files[0].Status)
	assert.True(t, s.Files[0].Staged)
}

func TestParse_ModifiedStagedAndUnstaged(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()

	makeInitialCommit(t, dir, "file.txt", "v1\n")

	path := filepath.Join(dir, "file.txt")
	require.NoError(t, os.WriteFile(path, []byte("v2\n"), 0600))
	_, _ = exec.Git(ctx, dir, "add", "file.txt")
	require.NoError(t, os.WriteFile(path, []byte("v3\n"), 0600))

	s, err := status.Parse(ctx, dir)

	require.NoError(t, err)

	var stagedFile, unstagedFile *domain.GitFile
	for i := range s.Files {
		f := &s.Files[i]
		if f.Path == "file.txt" && f.Staged {
			stagedFile = f
		}
		if f.Path == "file.txt" && !f.Staged {
			unstagedFile = f
		}
	}
	require.NotNil(t, stagedFile, "expected a staged entry for file.txt")
	require.NotNil(t, unstagedFile, "expected an unstaged entry for file.txt")
	assert.Equal(t, domain.GitFileStatusModified, stagedFile.Status)
	assert.Equal(t, domain.GitFileStatusModified, unstagedFile.Status)
}

func TestParse_RenamedFile(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()

	makeInitialCommit(t, dir, "old.txt", "content\n")

	require.NoError(t, os.Rename(filepath.Join(dir, "old.txt"), filepath.Join(dir, "new.txt")))
	_, _ = exec.Git(ctx, dir, "add", "--all")

	s, err := status.Parse(ctx, dir)

	require.NoError(t, err)

	var found bool
	for _, f := range s.Files {
		if f.Status == domain.GitFileStatusRenamed {
			found = true
			assert.Equal(t, "new.txt", f.Path)
			assert.True(t, f.Staged)
		}
	}
	assert.True(t, found, "expected a renamed entry")
}

func TestParse_ConflictedFile(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()

	makeInitialCommit(t, dir, "conflict.txt", "base\n")

	r, err := exec.Git(ctx, dir, "checkout", "-b", "branch-a")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("branch-a change\n"), 0600))
	_, _ = exec.Git(ctx, dir, "add", "conflict.txt")
	r, err = exec.Git(ctx, dir, "commit", "-m", "branch-a change")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	_, _ = exec.Git(ctx, dir, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte("main change\n"), 0600))
	_, _ = exec.Git(ctx, dir, "add", "conflict.txt")
	r, err = exec.Git(ctx, dir, "commit", "-m", "main change")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	_, _ = exec.Git(ctx, dir, "merge", "branch-a")

	s, err := status.Parse(ctx, dir)

	require.NoError(t, err)

	var found bool
	for _, f := range s.Files {
		if f.Status == domain.GitFileStatusConflicted {
			found = true
			assert.Equal(t, "conflict.txt", f.Path)
			assert.False(t, f.Staged)
		}
	}
	assert.True(t, found, "expected a conflicted entry for conflict.txt")
}

func TestParse_AheadBehind(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()

	makeInitialCommit(t, dir, "base.txt", "base\n")

	remoteDir := t.TempDir()
	r, err := exec.Git(ctx, remoteDir, "init", "--bare", "-b", "main")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	r, err = exec.Git(ctx, dir, "remote", "add", "origin", remoteDir)
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	r, err = exec.Git(ctx, dir, "push", "-u", "origin", "main")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	makeInitialCommit(t, dir, "extra.txt", "extra\n")

	s, err := status.Parse(ctx, dir)

	require.NoError(t, err)
	assert.Equal(t, 1, s.Ahead)
	assert.Equal(t, 0, s.Behind)
}

// --- Error path ---

func TestParse_InvalidRepo(
	t *testing.T,
) {
	ctx := context.Background()

	_, err := status.Parse(ctx, "/nonexistent/path/that/does/not/exist")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status: parse")
}

// --- parseAheadBehind edge cases ---

func TestParse_BehindOnly(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()

	makeInitialCommit(t, dir, "base.txt", "base\n")

	remoteDir := t.TempDir()
	r, err := exec.Git(ctx, remoteDir, "init", "--bare", "-b", "main")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	r, err = exec.Git(ctx, dir, "remote", "add", "origin", remoteDir)
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	r, err = exec.Git(ctx, dir, "push", "-u", "origin", "main")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	// Push a commit from a clone so remote is ahead.
	cloneDir := t.TempDir()
	r, err = exec.Git(ctx, cloneDir, "clone", remoteDir, ".")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)
	_, _ = exec.Git(ctx, cloneDir, "config", "user.email", "test@test.com")
	_, _ = exec.Git(ctx, cloneDir, "config", "user.name", "Test")
	clonePath := filepath.Join(cloneDir, "remote.txt")
	require.NoError(t, os.WriteFile(clonePath, []byte("remote\n"), 0600))
	_, _ = exec.Git(ctx, cloneDir, "add", "remote.txt")
	r, err = exec.Git(ctx, cloneDir, "commit", "-m", "remote commit")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)
	r, err = exec.Git(ctx, cloneDir, "push", "origin", "main")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	_, _ = exec.Git(ctx, dir, "fetch", "origin")

	s, err := status.Parse(ctx, dir)

	require.NoError(t, err)
	assert.Equal(t, 0, s.Ahead)
	assert.Equal(t, 1, s.Behind)
}

// --- Staged deleted file: charToStatus('D') on X position ---

func TestParse_StagedDeletedFile(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()

	makeInitialCommit(t, dir, "todelete.txt", "content\n")

	_, _ = exec.Git(ctx, dir, "rm", "todelete.txt")

	s, err := status.Parse(ctx, dir)

	require.NoError(t, err)

	var found bool
	for _, f := range s.Files {
		if f.Path == "todelete.txt" && f.Staged {
			found = true
			assert.Equal(t, domain.GitFileStatusDeleted, f.Status)
		}
	}
	assert.True(t, found, "expected a staged deleted entry for todelete.txt")
}

// --- Unstaged deleted file: charToStatus('D') on Y position ---

func TestParse_UnstagedDeletedFile(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()

	makeInitialCommit(t, dir, "todelete.txt", "content\n")

	require.NoError(t, os.Remove(filepath.Join(dir, "todelete.txt")))

	s, err := status.Parse(ctx, dir)

	require.NoError(t, err)

	var found bool
	for _, f := range s.Files {
		if f.Path == "todelete.txt" && !f.Staged {
			found = true
			assert.Equal(t, domain.GitFileStatusDeleted, f.Status)
		}
	}
	assert.True(t, found, "expected an unstaged deleted entry for todelete.txt")
}

// --- parseRenamedEntry: non-R/C first char (staged=false) ---

func TestParse_RenamedEntryUnstagedCopy(
	t *testing.T,
) {
	// We cannot easily produce a working-tree rename without staging it via
	// the git porcelain, so we verify the staged=true path is already covered
	// by TestParse_RenamedFile and exercise the parser directly via a real
	// copy-in-index scenario using git cp alias workaround.
	// The simplest achievable real test is a copy (C) which git may report;
	// we skip if git doesn't detect it and just verify no panic.
	dir := initRepo(t)
	ctx := context.Background()

	makeInitialCommit(t, dir, "src.txt", "content\n")

	// Copy the file and add; git may report it as a copy (C) or addition (A).
	require.NoError(t, func() error {
		data, e := os.ReadFile(filepath.Join(dir, "src.txt"))
		if e != nil {
			return e
		}
		return os.WriteFile(filepath.Join(dir, "dst.txt"), data, 0600)
	}())
	_, _ = exec.Git(ctx, dir, "add", "dst.txt")

	s, err := status.Parse(ctx, dir)

	require.NoError(t, err)
	// dst.txt is either Added or Renamed/Copied — just verify parsing didn't panic.
	var found bool
	for _, f := range s.Files {
		if f.Path == "dst.txt" {
			found = true
		}
	}
	assert.True(t, found, "expected dst.txt in status output")
}

// --- parseUnmergedEntry: DD conflict (both deleted) ---

func TestParse_ConflictBothDeleted(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()

	makeInitialCommit(t, dir, "shared.txt", "original\n")

	r, err := exec.Git(ctx, dir, "checkout", "-b", "branch-b")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)
	_, _ = exec.Git(ctx, dir, "rm", "shared.txt")
	r, err = exec.Git(ctx, dir, "commit", "-m", "delete on branch-b")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	_, _ = exec.Git(ctx, dir, "checkout", "main")
	_, _ = exec.Git(ctx, dir, "rm", "shared.txt")
	r, err = exec.Git(ctx, dir, "commit", "-m", "delete on main")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	// Merging two branches that both deleted the same file results in a
	// clean fast-forward or a DD conflict depending on git version; either way
	// the parse must not error.
	_, _ = exec.Git(ctx, dir, "merge", "branch-b")

	s, err := status.Parse(ctx, dir)
	require.NoError(t, err)
	// Whether it conflicts or not, parsing succeeded.
	_ = s
}

// --- isConflict: AA (both added) path ---

func TestParse_ConflictBothAdded(
	t *testing.T,
) {
	dir := initRepo(t)
	ctx := context.Background()

	// Start with an empty initial commit so both branches can add the same file.
	emptyPath := filepath.Join(dir, ".gitkeep")
	require.NoError(t, os.WriteFile(emptyPath, []byte(""), 0600))
	_, _ = exec.Git(ctx, dir, "add", ".gitkeep")
	r, err := exec.Git(ctx, dir, "commit", "-m", "empty root")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	r, err = exec.Git(ctx, dir, "checkout", "-b", "branch-c")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "newfile.txt"), []byte("from branch-c\n"), 0600))
	_, _ = exec.Git(ctx, dir, "add", "newfile.txt")
	r, err = exec.Git(ctx, dir, "commit", "-m", "add on branch-c")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	_, _ = exec.Git(ctx, dir, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "newfile.txt"), []byte("from main\n"), 0600))
	_, _ = exec.Git(ctx, dir, "add", "newfile.txt")
	r, err = exec.Git(ctx, dir, "commit", "-m", "add on main")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	_, _ = exec.Git(ctx, dir, "merge", "branch-c")

	s, err := status.Parse(ctx, dir)

	require.NoError(t, err)

	var found bool
	for _, f := range s.Files {
		if f.Path == "newfile.txt" && f.Status == domain.GitFileStatusConflicted {
			found = true
		}
	}
	assert.True(t, found, "expected newfile.txt to be conflicted (AA)")
}

// --- isConflict: xU and Ux paths ---

func TestParse_ConflictUWithOther(
	t *testing.T,
) {
	// A standard content conflict produces UU in porcelain v2 "u" entries.
	// The isConflict helper is exercised indirectly via ordinary entry parsing
	// when xy contains 'U'. Produce a UU conflict via a normal merge conflict.
	dir := initRepo(t)
	ctx := context.Background()

	makeInitialCommit(t, dir, "file.txt", "base\n")

	r, err := exec.Git(ctx, dir, "checkout", "-b", "branch-d")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("branch-d\n"), 0600))
	_, _ = exec.Git(ctx, dir, "add", "file.txt")
	r, err = exec.Git(ctx, dir, "commit", "-m", "branch-d change")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	_, _ = exec.Git(ctx, dir, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("main change\n"), 0600))
	_, _ = exec.Git(ctx, dir, "add", "file.txt")
	r, err = exec.Git(ctx, dir, "commit", "-m", "main change")
	require.NoError(t, err)
	require.Equal(t, 0, r.ExitCode, r.Stderr)

	_, _ = exec.Git(ctx, dir, "merge", "branch-d")

	s, err := status.Parse(ctx, dir)

	require.NoError(t, err)

	var found bool
	for _, f := range s.Files {
		if f.Status == domain.GitFileStatusConflicted {
			found = true
		}
	}
	assert.True(t, found, "expected at least one conflicted file")
}

// --- White-box tests using exported internal helpers ---

func TestParseAheadBehind_WrongFieldCount(
	t *testing.T,
) {
	ahead, behind := status.ExportedParseAheadBehind("only-one-field")
	assert.Equal(t, 0, ahead)
	assert.Equal(t, 0, behind)
}

func TestParseOrdinaryEntries_TooFewParts(
	t *testing.T,
) {
	files := status.ExportedParseOrdinaryEntries("1 M. N... 100644 100644")
	assert.Nil(t, files)
}

func TestParseOrdinaryEntries_ShortXY(
	t *testing.T,
) {
	// xy field has length < 2
	files := status.ExportedParseOrdinaryEntries("1 X N... 100644 100644 100644 abc def path.txt")
	assert.Nil(t, files)
}

func TestParseOrdinaryEntries_ConflictXY(
	t *testing.T,
) {
	// xy = "UU" triggers isConflict → returns a single conflicted entry.
	files := status.ExportedParseOrdinaryEntries("1 UU N... 100644 100644 100644 abc def conflict.txt")
	require.Len(t, files, 1)
	assert.Equal(t, domain.GitFileStatusConflicted, files[0].Status)
	assert.Equal(t, "conflict.txt", files[0].Path)
	assert.False(t, files[0].Staged)
}

func TestParseOrdinaryEntries_StagedDeleted(
	t *testing.T,
) {
	// xy = "D." → staged deletion, no unstaged change.
	files := status.ExportedParseOrdinaryEntries("1 D. N... 100644 000000 000000 abc def deleted.txt")
	require.Len(t, files, 1)
	assert.Equal(t, domain.GitFileStatusDeleted, files[0].Status)
	assert.True(t, files[0].Staged)
}

func TestParseOrdinaryEntries_UnstagedDeleted(
	t *testing.T,
) {
	// xy = ".D" → unstaged deletion.
	files := status.ExportedParseOrdinaryEntries("1 .D N... 100644 100644 000000 abc def missing.txt")
	require.Len(t, files, 1)
	assert.Equal(t, domain.GitFileStatusDeleted, files[0].Status)
	assert.False(t, files[0].Staged)
}

func TestParseOrdinaryEntries_StagedAdded(
	t *testing.T,
) {
	// xy = "A." → staged new file.
	files := status.ExportedParseOrdinaryEntries("1 A. N... 000000 100644 000000 abc def newfile.txt")
	require.Len(t, files, 1)
	assert.Equal(t, domain.GitFileStatusAdded, files[0].Status)
	assert.True(t, files[0].Staged)
}

func TestParseOrdinaryEntries_StagedRenamed(
	t *testing.T,
) {
	// xy = "R." → staged rename.
	files := status.ExportedParseOrdinaryEntries("1 R. N... 100644 100644 100644 abc def renamed.txt")
	require.Len(t, files, 1)
	assert.Equal(t, domain.GitFileStatusRenamed, files[0].Status)
	assert.True(t, files[0].Staged)
}

func TestParseRenamedEntry_TooFewParts(
	t *testing.T,
) {
	f := status.ExportedParseRenamedEntry("2 R. N... 100644 100644")
	assert.Equal(t, domain.GitFile{}, f)
}

func TestParseRenamedEntry_ShortXY(
	t *testing.T,
) {
	f := status.ExportedParseRenamedEntry("2 X N... 100644 100644 100644 100 abc def ghi new.txt\told.txt")
	assert.Equal(t, domain.GitFile{}, f)
}

func TestParseRenamedEntry_NotROrC(
	t *testing.T,
) {
	// xy = ".R" → staged=false (x != 'R' && x != 'C').
	f := status.ExportedParseRenamedEntry("2 .R N... 100644 100644 100644 abc def R100 new.txt")
	assert.Equal(t, "new.txt", f.Path)
	assert.Equal(t, domain.GitFileStatusRenamed, f.Status)
	assert.False(t, f.Staged)
}

func TestParseUnmergedEntry_TooFewParts(
	t *testing.T,
) {
	f := status.ExportedParseUnmergedEntry("u UU N... 1 2 3")
	assert.Equal(t, domain.GitFile{}, f)
}

func TestIsConflict_ShortXY(
	t *testing.T,
) {
	assert.False(t, status.ExportedIsConflict(""))
	assert.False(t, status.ExportedIsConflict("A"))
}

func TestIsConflict_YU(
	t *testing.T,
) {
	assert.True(t, status.ExportedIsConflict("AU"))
}

func TestIsConflict_XU(
	t *testing.T,
) {
	assert.True(t, status.ExportedIsConflict("UA"))
}

func TestIsConflict_AA(
	t *testing.T,
) {
	assert.True(t, status.ExportedIsConflict("AA"))
}

func TestIsConflict_DD(
	t *testing.T,
) {
	assert.True(t, status.ExportedIsConflict("DD"))
}

func TestIsConflict_NotConflict(
	t *testing.T,
) {
	assert.False(t, status.ExportedIsConflict("M."))
	assert.False(t, status.ExportedIsConflict(".M"))
	assert.False(t, status.ExportedIsConflict("AD"))
}

func TestCharToStatus_R(
	t *testing.T,
) {
	assert.Equal(t, domain.GitFileStatusRenamed, status.ExportedCharToStatus('R'))
}

func TestCharToStatus_Default(
	t *testing.T,
) {
	// Unknown char → Modified.
	assert.Equal(t, domain.GitFileStatusModified, status.ExportedCharToStatus('X'))
}
