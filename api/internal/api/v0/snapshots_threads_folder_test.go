package v0

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/usecases/folder"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestThreadsSnapshot_NoWorkspaceSegmentReturnsNil covers the guard at the top
// of threadsSnapshot: threads are always workspace-scoped, so a repo- or
// project-level subscription (fewer than 3 scope segments, or an empty
// workspace segment) must yield nil rather than attempting a global
// enumeration of the ReviewThread aggregate.
func TestThreadsSnapshot_NoWorkspaceSegmentReturnsNil(t *testing.T) {
	a := newAppForSnapshot(t)
	snap := threadsSnapshot(a)
	require.NotNil(t, snap)

	assert.Nil(t, snap(""))
	assert.Nil(t, snap("p1"))
	assert.Nil(t, snap("p1/r1"))
	assert.Nil(t, snap("p1/r1/"))
}

// errReviewThreadRepo is a ReviewThread repo whose ListByWorkspace always
// fails, exercising threadsSnapshot's degrade-to-nil path.
type errReviewThreadRepo struct {
	reviewthread.ReviewThread
}

func (errReviewThreadRepo) ListByWorkspace(
	_ context.Context,
	_ string,
) ([]domain.ReviewThread, error) {
	return nil, errSnapshotFake
}

func TestThreadsSnapshot_ListErrorReturnsNil(t *testing.T) {
	a := newAppForSnapshot(t)
	a.Repositories.ReviewThread = errReviewThreadRepo{}

	assert.Nil(t, threadsSnapshot(a)("p1/r1/w1"))
}

// errFolderUsecase is a Folder usecase whose ListInRepo always fails,
// exercising folderSnapshot's degrade-to-nil path.
type errFolderUsecase struct {
	folder.Usecase
}

func (errFolderUsecase) ListInRepo(
	_ context.Context,
	_ string,
	_ string,
) ([]domain.Folder, error) {
	return nil, errSnapshotFake
}

func TestFolderSnapshot_ListErrorReturnsNil(t *testing.T) {
	a := newAppForSnapshot(t)
	a.Usecases.Folder = errFolderUsecase{}

	assert.Nil(t, folderSnapshot(a)("p1/r1"))
}
