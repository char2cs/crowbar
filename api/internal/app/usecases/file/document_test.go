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
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

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

func TestReadDocument_LongestWorktreePrefixWins(t *testing.T) {
	fs, syncer, uc := newFileUsecase(t)
	withRows(syncer,
		domain.Workspace{ID: "home", Kind: domain.WorkspaceKindHome, WorktreePath: "/p"},
		domain.Workspace{ID: "w1", WorktreePath: "/p/repo/wt"},
	)
	got := recordReads(fs)

	doc, err := uc.ReadDocument(context.Background(), domain.DocumentRef{Path: "/p/repo/wt/src/a.go"}, domain.TransportRemote)

	require.NoError(t, err)
	assert.Equal(t, workspaceRef("w1", "src/a.go"), doc.DocumentRef)
	assert.Equal(t, [2]string{"/p/repo/wt", "src/a.go"}, *got)
	assert.Equal(t, "hi", doc.Content)
}

func TestReadDocument_HomeWorkspaceRowMatches(t *testing.T) {
	fs, syncer, uc := newFileUsecase(t)
	withRows(syncer, domain.Workspace{ID: "home", Kind: domain.WorkspaceKindHome, WorktreePath: "/p"})
	recordReads(fs)

	doc, err := uc.ReadDocument(context.Background(), domain.DocumentRef{Path: "/p/notes.md"}, domain.TransportRemote)

	require.NoError(t, err)
	assert.Equal(t, workspaceRef("home", "notes.md"), doc.DocumentRef)
}

// TestReadDocument_RepoRowBeatsTheProjectHomeAtTheSamePath pins the tie: a
// project imported in place has its home and its repo at one path, and the
// file belongs to the repo whatever order the rows list in.
func TestReadDocument_RepoRowBeatsTheProjectHomeAtTheSamePath(t *testing.T) {
	fs, syncer, uc := newFileUsecase(t)
	withRows(syncer,
		domain.Workspace{ID: "a-home", Kind: domain.WorkspaceKindHome, WorktreePath: "/p"},
		domain.Workspace{ID: "b-repo", WorktreePath: "/p"},
	)
	recordReads(fs)

	doc, err := uc.ReadDocument(context.Background(), domain.DocumentRef{Path: "/p/a.md"}, domain.TransportRemote)

	require.NoError(t, err)
	assert.Equal(t, workspaceRef("b-repo", "a.md"), doc.DocumentRef)
}

func TestReadDocument_PlaceholderRowsClaimNothing(t *testing.T) {
	fs, syncer, uc := newFileUsecase(t)
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
	fs, syncer, uc := newFileUsecase(t)
	wt := t.TempDir()
	link := filepath.Join(t.TempDir(), "wt-link")
	require.NoError(t, os.Symlink(wt, link))
	withRows(syncer, domain.Workspace{ID: "w1", WorktreePath: wt})
	recordReads(fs)

	doc, err := uc.ReadDocument(context.Background(), domain.DocumentRef{Path: filepath.Join(link, "a.md")}, domain.TransportRemote)

	require.NoError(t, err)
	assert.Equal(t, workspaceRef("w1", "a.md"), doc.DocumentRef)
}

// TestReadDocument_FoldsCaseOnDarwin pins one identity per file on the default
// case-insensitive APFS: a differently cased path into a worktree is still the
// workspace document there, and stays a distinct host path elsewhere.
func TestReadDocument_FoldsCaseOnDarwin(t *testing.T) {
	fs, syncer, uc := newFileUsecase(t)
	withRows(syncer, domain.Workspace{ID: "w1", WorktreePath: "/repo/wt"})
	recordReads(fs)
	fs.ReadHostContentFn = func(string) (domain.FileContent, error) { return domain.FileContent{}, nil }

	doc, err := uc.ReadDocument(context.Background(), domain.DocumentRef{Path: "/Repo/WT/A.md"}, domain.TransportLocal)

	require.NoError(t, err)
	if runtime.GOOS == "darwin" {
		assert.Equal(t, workspaceRef("w1", "A.md"), doc.DocumentRef)
		return
	}
	assert.Equal(t, domain.DocumentAuthorityHost, doc.Authority)
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
	fs, syncer, uc := newFileUsecase(t)
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
	fs, syncer, uc := newFileUsecase(t)
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
	_, syncer, uc := newFileUsecase(t)
	withRows(syncer, domain.Workspace{ID: "home", Kind: domain.WorkspaceKindHome, WorktreePath: testHome})
	ctx := context.Background()
	target := domain.DocumentRef{Path: filepath.Join(testHome, "state", "global.db")}

	_, err := uc.ReadDocument(ctx, target, domain.TransportLocal)
	require.ErrorIs(t, err, apperr.ErrDaemonStateDocument)
	_, err = uc.WriteDocument(ctx, target, "x", "", domain.TransportLocal, time.Now())
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
