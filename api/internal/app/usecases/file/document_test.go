package file_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// newDocumentUsecase builds the usecase over a real daemon home, whose state
// directory the guard compares paths against by file identity.
func newDocumentUsecase(
	t *testing.T,
) (*mocks.FsEngine, *mocks.WorkspaceSyncer, file.Usecase, string) {
	t.Helper()
	home := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(home, "state"), 0o700))
	fs := mocks.NewFsEngine()
	syncer := mocks.NewWorkspaceSyncer()
	return fs, syncer, file.New(fs, syncer, func() (string, error) { return home, nil }), home
}

// withRows makes rows the syncer's whole workspace list, each also readable by id.
func withRows(
	syncer *mocks.WorkspaceSyncer,
	rows ...domain.Workspace,
) {
	syncer.ListFn = func(_ context.Context) ([]domain.Workspace, error) { return rows, nil }
	syncer.GetFn = func(_ context.Context, id string) (domain.Workspace, error) {
		for _, ws := range rows {
			if ws.ID == id {
				return ws, nil
			}
		}
		return domain.Workspace{}, apperr.ErrNotFound
	}
}

// recordReads makes the fake fs answer every workspace read and remember where it read.
func recordReads(
	fs *mocks.FsEngine,
) *[2]string {
	var got [2]string
	fs.ReadContentFn = func(repoPath, filePath string) (domain.FileContent, error) {
		got = [2]string{repoPath, filePath}
		return domain.FileContent{Content: "hi"}, nil
	}
	return &got
}

func workspaceRef(
	wsID string,
	path string,
) domain.DocumentRef {
	return domain.DocumentRef{Authority: domain.DocumentAuthorityWorkspace, WorkspaceID: wsID, Path: path}
}

// mkdir makes the directory parts joined under base and answers its path.
func mkdir(
	t *testing.T,
	base string,
	parts ...string,
) string {
	t.Helper()
	dir := filepath.Join(append([]string{base}, parts...)...)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	return dir
}

func TestReadDocument_NearestWorktreeWins(t *testing.T) {
	fs, syncer, uc, _ := newDocumentUsecase(t)
	p := t.TempDir()
	wt := mkdir(t, p, "repo", "wt")
	withRows(syncer,
		domain.Workspace{ID: "home", Kind: domain.WorkspaceKindHome, WorktreePath: p},
		domain.Workspace{ID: "w1", WorktreePath: wt},
	)
	got := recordReads(fs)

	doc, err := uc.ReadDocument(context.Background(), domain.DocumentRef{Path: filepath.Join(wt, "src", "a.go")}, domain.TransportRemote)

	require.NoError(t, err)
	assert.Equal(t, workspaceRef("w1", "src/a.go"), doc.DocumentRef)
	assert.Equal(t, [2]string{wt, "src/a.go"}, *got)
	assert.Equal(t, "hi", doc.Content)
}

func TestReadDocument_HomeWorkspaceRowMatches(t *testing.T) {
	fs, syncer, uc, _ := newDocumentUsecase(t)
	p := t.TempDir()
	withRows(syncer, domain.Workspace{ID: "home", Kind: domain.WorkspaceKindHome, WorktreePath: p})
	recordReads(fs)

	doc, err := uc.ReadDocument(context.Background(), domain.DocumentRef{Path: filepath.Join(p, "notes.md")}, domain.TransportRemote)

	require.NoError(t, err)
	assert.Equal(t, workspaceRef("home", "notes.md"), doc.DocumentRef)
}

// TestReadDocument_RepoRowBeatsTheProjectHomeAtTheSamePath pins the tie: a
// project imported in place has its home and its repo at one path, and the
// file belongs to the repo whatever order the rows list in.
func TestReadDocument_RepoRowBeatsTheProjectHomeAtTheSamePath(t *testing.T) {
	fs, syncer, uc, _ := newDocumentUsecase(t)
	p := t.TempDir()
	withRows(syncer,
		domain.Workspace{ID: "a-home", Kind: domain.WorkspaceKindHome, WorktreePath: p},
		domain.Workspace{ID: "b-repo", WorktreePath: p},
	)
	recordReads(fs)

	doc, err := uc.ReadDocument(context.Background(), domain.DocumentRef{Path: filepath.Join(p, "a.md")}, domain.TransportRemote)

	require.NoError(t, err)
	assert.Equal(t, workspaceRef("b-repo", "a.md"), doc.DocumentRef)
}

func TestReadDocument_PlaceholderRowsClaimNothing(t *testing.T) {
	fs, syncer, uc, _ := newDocumentUsecase(t)
	withRows(syncer, domain.Workspace{ID: "ph", Status: domain.WorkspaceStatusLocked})
	fs.ReadHostContentFn = func(absPath string) (domain.FileContent, error) {
		return domain.FileContent{Content: absPath}, nil
	}

	doc, err := uc.ReadDocument(context.Background(), domain.DocumentRef{Path: "/elsewhere/a.md"}, domain.TransportLocal)

	require.NoError(t, err)
	assert.Equal(t, domain.DocumentRef{Authority: domain.DocumentAuthorityHost, Path: "/elsewhere/a.md"}, doc.DocumentRef)
}

func TestReadDocument_SymlinkIntoWorktreeIsTheWorkspaceDocument(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	fs, syncer, uc, _ := newDocumentUsecase(t)
	wt := t.TempDir()
	link := filepath.Join(t.TempDir(), "wt-link")
	require.NoError(t, os.Symlink(wt, link))
	withRows(syncer, domain.Workspace{ID: "w1", WorktreePath: wt})
	recordReads(fs)

	doc, err := uc.ReadDocument(context.Background(), domain.DocumentRef{Path: filepath.Join(link, "a.md")}, domain.TransportRemote)

	require.NoError(t, err)
	assert.Equal(t, workspaceRef("w1", "a.md"), doc.DocumentRef)
}

// TestReadDocument_SpellsThePathAsOnDisk pins one identity per file on a
// case-insensitive volume: another casing of a worktree file is that file,
// spelled as the tree spells it.
func TestReadDocument_SpellsThePathAsOnDisk(t *testing.T) {
	fs, syncer, uc, _ := newDocumentUsecase(t)
	p := t.TempDir()
	wt := mkdir(t, p, "repo", "wt")
	require.NoError(t, os.WriteFile(filepath.Join(wt, "a.md"), nil, 0o600))
	shouted := filepath.Join(p, "Repo", "WT", "A.md")
	if _, err := os.Stat(shouted); err != nil {
		t.Skip("case-sensitive volume")
	}
	withRows(syncer, domain.Workspace{ID: "w1", WorktreePath: wt})
	recordReads(fs)

	doc, err := uc.ReadDocument(context.Background(), domain.DocumentRef{Path: shouted}, domain.TransportRemote)

	require.NoError(t, err)
	assert.Equal(t, workspaceRef("w1", "a.md"), doc.DocumentRef)
}

// TestDocument_FirmlinkAliasHasTheOneIdentity pins the alias symlink resolution
// leaves alone: on darwin /System/Volumes/Data<path> is <path> itself, so the
// state stays refused and a worktree file keeps its lock and its resync.
func TestDocument_FirmlinkAliasHasTheOneIdentity(t *testing.T) {
	fs, syncer, uc, home := newDocumentUsecase(t)
	wt := t.TempDir()
	alias := func(p string) string {
		canon, err := filepath.EvalSymlinks(p)
		require.NoError(t, err)
		return "/System/Volumes/Data" + canon
	}
	if _, err := os.Stat(alias(wt)); err != nil {
		t.Skip("no /System/Volumes/Data firmlink to the temp dir on this host")
	}
	ctx := context.Background()
	state := domain.DocumentRef{Path: filepath.Join(alias(home), "state", "global.db")}
	_, err := uc.ReadDocument(ctx, state, domain.TransportLocal)
	require.ErrorIs(t, err, apperr.ErrDaemonStateDocument)
	_, err = uc.WriteDocument(ctx, state, "x", "", domain.TransportLocal, time.Now())
	require.ErrorIs(t, err, apperr.ErrDaemonStateDocument)

	doc := domain.DocumentRef{Path: filepath.Join(alias(wt), "a.md")}
	withRows(syncer, domain.Workspace{ID: "w1", WorktreePath: wt, Status: domain.WorkspaceStatusLocked})
	_, err = uc.WriteDocument(ctx, doc, "x", "", domain.TransportLocal, time.Now())
	require.ErrorIs(t, err, apperr.ErrLocked)

	withRows(syncer, domain.Workspace{ID: "w1", WorktreePath: wt})
	fs.WriteContentFn = func(_, _, _, _ string) error { return nil }
	ref, err := uc.WriteDocument(ctx, doc, "x", "", domain.TransportLocal, time.Now())
	require.NoError(t, err)
	assert.Equal(t, workspaceRef("w1", "a.md"), ref)
	assert.Equal(t, "w1", syncer.SyncedID)
}

func TestDocument_RelativePathWithoutWorkspaceIsABadRequest(t *testing.T) {
	_, _, uc := newFileUsecase(t)
	ctx := context.Background()

	_, err := uc.ReadDocument(ctx, domain.DocumentRef{Path: "a.md"}, domain.TransportLocal)
	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
	_, err = uc.WriteDocument(ctx, domain.DocumentRef{Path: "a.md"}, "x", "", domain.TransportLocal, time.Now())
	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

func TestReadDocument_HostNeedsTheLocalTransport(t *testing.T) {
	fs, syncer, uc, _ := newDocumentUsecase(t)
	withRows(syncer)
	var read string
	fs.ReadHostContentFn = func(absPath string) (domain.FileContent, error) {
		read = absPath
		return domain.FileContent{Content: "outside"}, nil
	}
	target := domain.DocumentRef{Path: "/elsewhere/a.md"}

	_, err := uc.ReadDocument(context.Background(), target, domain.TransportRemote)
	require.ErrorIs(t, err, apperr.ErrHostDocumentNeedsLocalTransport)
	assert.Empty(t, read, "a refused read must never touch the disk")

	doc, err := uc.ReadDocument(context.Background(), target, domain.TransportLocal)
	require.NoError(t, err)
	assert.Equal(t, "/elsewhere/a.md", read)
	assert.Equal(t, domain.Document{
		DocumentRef: domain.DocumentRef{Authority: domain.DocumentAuthorityHost, Path: "/elsewhere/a.md"},
		FileContent: domain.FileContent{Content: "outside"},
	}, doc)
}

func TestWriteDocument_HostNeedsTheLocalTransportAndNeverResyncs(t *testing.T) {
	fs, syncer, uc, _ := newDocumentUsecase(t)
	withRows(syncer)
	var wrote []string
	fs.WriteHostContentFn = func(absPath, content, encoding string) error {
		wrote = append(wrote, absPath, content, encoding)
		return nil
	}
	target := domain.DocumentRef{Authority: domain.DocumentAuthorityHost, Path: "/elsewhere/a.bin"}

	_, err := uc.WriteDocument(context.Background(), target, "AA==", "base64", domain.TransportRemote, time.Now())
	require.ErrorIs(t, err, apperr.ErrHostDocumentNeedsLocalTransport)
	assert.Empty(t, wrote)

	ref, err := uc.WriteDocument(context.Background(), target, "AA==", "base64", domain.TransportLocal, time.Now())
	require.NoError(t, err)
	assert.Equal(t, target, ref)
	assert.Equal(t, []string{"/elsewhere/a.bin", "AA==", "base64"}, wrote)
	assert.False(t, syncer.Synced, "a host file belongs to no working tree")
}

func TestDocument_DaemonStateIsRefusedEvenLocally(t *testing.T) {
	_, syncer, uc, home := newDocumentUsecase(t)
	withRows(syncer, domain.Workspace{ID: "home", Kind: domain.WorkspaceKindHome, WorktreePath: home})
	ctx := context.Background()
	target := domain.DocumentRef{Path: filepath.Join(home, "state", "global.db")}

	_, err := uc.ReadDocument(ctx, target, domain.TransportLocal)
	require.ErrorIs(t, err, apperr.ErrDaemonStateDocument)
	_, err = uc.WriteDocument(ctx, target, "x", "", domain.TransportLocal, time.Now())
	require.ErrorIs(t, err, apperr.ErrDaemonStateDocument)
}

// TestDocument_DanglingLinkInStateIsRefused pins the guard on a path no
// symlink resolution spells canonically: a dangling link inside state, reached
// through an alias of the home.
func TestDocument_DanglingLinkInStateIsRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	_, syncer, uc, home := newDocumentUsecase(t)
	withRows(syncer)
	require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "gone"), filepath.Join(home, "state", "global.db-journal")))
	alias := filepath.Join(t.TempDir(), "home")
	require.NoError(t, os.Symlink(home, alias))
	target := domain.DocumentRef{Path: filepath.Join(alias, "state", "global.db-journal")}

	_, err := uc.WriteDocument(context.Background(), target, "x", "", domain.TransportLocal, time.Now())
	require.ErrorIs(t, err, apperr.ErrDaemonStateDocument)
}

func TestWriteDocument_WorkspaceWritesThenResyncs(t *testing.T) {
	fs, syncer, uc := newFileUsecase(t)
	okWorkspace(syncer)
	var wrote []string
	fs.WriteContentFn = func(repoPath, filePath, content, encoding string) error {
		wrote = []string{repoPath, filePath, content, encoding}
		return nil
	}

	ref, err := uc.WriteDocument(context.Background(), workspaceRef("w1", "a.go"), "data", "base64", domain.TransportRemote, time.Unix(1000, 0))

	require.NoError(t, err)
	assert.Equal(t, workspaceRef("w1", "a.go"), ref)
	assert.Equal(t, []string{"/repo/x", "a.go", "data", "base64"}, wrote, "encoding must reach the fs engine unchanged")
	assert.Equal(t, "w1", syncer.SyncedID)
}

func TestWriteDocument_WorkspaceFailuresNeverResync(t *testing.T) {
	ctx := context.Background()
	fs, syncer, uc := newFileUsecase(t)
	okWorkspace(syncer)
	fs.WriteContentFn = func(_, _, _, _ string) error { return errors.New("boom") }
	_, err := uc.WriteDocument(ctx, workspaceRef("w1", "a.go"), "d", "", domain.TransportLocal, time.Now())
	require.Error(t, err)
	assert.False(t, syncer.Synced)

	fs, syncer, uc = newFileUsecase(t)
	lockedWorkspace(syncer)
	fs.WriteContentFn = func(_, _, _, _ string) error { return nil }
	_, err = uc.WriteDocument(ctx, workspaceRef("w1", "a.go"), "d", "", domain.TransportLocal, time.Now())
	require.ErrorIs(t, err, apperr.ErrLocked)
	assert.False(t, syncer.Synced, "locked rejection must not mutate or resync")
}

func TestWriteDocument_ResyncError(t *testing.T) {
	fs, syncer, uc := newFileUsecase(t)
	okWorkspace(syncer)
	syncer.SyncFn = func(_ context.Context, _ string, _ time.Time) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("boom")
	}
	fs.WriteContentFn = func(_, _, _, _ string) error { return nil }

	_, err := uc.WriteDocument(context.Background(), workspaceRef("w1", "a.go"), "d", "", domain.TransportLocal, time.Now())
	assert.Error(t, err)
}

func TestReadDocument_LockedWorkspaceStillReads(t *testing.T) {
	fs, syncer, uc := newFileUsecase(t)
	lockedWorkspace(syncer)
	recordReads(fs)

	doc, err := uc.ReadDocument(context.Background(), workspaceRef("w1", "a.go"), domain.TransportRemote)
	require.NoError(t, err)
	assert.Equal(t, "hi", doc.Content)
	assert.False(t, syncer.Synced)
}

func TestReadDocument_WorkspaceAndFsErrors(t *testing.T) {
	fs, syncer, uc := newFileUsecase(t)
	syncer.GetFn = func(_ context.Context, _ string) (domain.Workspace, error) {
		return domain.Workspace{}, errors.New("boom")
	}
	_, err := uc.ReadDocument(context.Background(), workspaceRef("w1", "a.go"), domain.TransportLocal)
	require.Error(t, err)

	okWorkspace(syncer)
	fs.ReadContentFn = func(_, _ string) (domain.FileContent, error) {
		return domain.FileContent{}, errors.New("boom")
	}
	_, err = uc.ReadDocument(context.Background(), workspaceRef("w1", "a.go"), domain.TransportLocal)
	require.Error(t, err)
}
