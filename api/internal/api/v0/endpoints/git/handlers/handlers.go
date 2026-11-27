package handlers

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type Git interface {
	Status(ctx context.Context, wsID string) (gitdomain.GitStatus, error)
	Diff(ctx context.Context, wsID string, staged bool) ([]gitdomain.FileDiff, error)
	Log(ctx context.Context, wsID string, limit, skip int) ([]gitdomain.Commit, error)
	Branches(ctx context.Context, wsID string) ([]gitdomain.Branch, error)
	Stashes(ctx context.Context, wsID string) ([]gitdomain.Stash, error)
	ConflictedFiles(ctx context.Context, wsID string) ([]string, error)
	ConflictHunks(ctx context.Context, wsID, filePath string) ([]gitdomain.ConflictHunk, error)
	CommitDiff(ctx context.Context, wsID, sha string) (gitdomain.MultiFileDiff, error)
	StageFile(ctx context.Context, wsID, filePath string, now time.Time) error
	StageHunk(ctx context.Context, wsID, filePath, hunkID string, now time.Time) error
	UnstageFile(ctx context.Context, wsID, filePath string, now time.Time) error
	UnstageHunk(ctx context.Context, wsID, filePath, hunkID string, now time.Time) error
	Discard(ctx context.Context, wsID, filePath string, now time.Time) error
	Commit(ctx context.Context, wsID, subject, body string, now time.Time) error
	Push(ctx context.Context, wsID string, now time.Time) error
	Fetch(ctx context.Context, wsID string, now time.Time) error
	Pull(ctx context.Context, wsID, mode string, now time.Time) error
	CreateBranch(ctx context.Context, wsID, name, source string, switchTo bool, now time.Time) error
	RenameBranch(ctx context.Context, wsID, oldName, newName string, now time.Time) error
	DeleteBranch(ctx context.Context, wsID, name string, now time.Time) error
	SwitchBranch(ctx context.Context, wsID, name string, now time.Time) error
	StashPush(ctx context.Context, wsID, message string, now time.Time) error
	StashApply(ctx context.Context, wsID, id string, now time.Time) error
	StashPop(ctx context.Context, wsID, id string, now time.Time) error
	StashDrop(ctx context.Context, wsID, id string, now time.Time) error
	Reset(ctx context.Context, wsID, mode, commit string, now time.Time) error
	Merge(ctx context.Context, wsID, branch string, now time.Time) error
	Rebase(ctx context.Context, wsID, onto string, now time.Time) error
	ResolveHunk(ctx context.Context, wsID, filePath, hunkID string, resolution gitdomain.ConflictResolution, resolvedContent string, now time.Time) error
	OperationContinue(ctx context.Context, wsID string, now time.Time) error
	OperationAbort(ctx context.Context, wsID string, now time.Time) error
}

type Operations interface {
	Start(
		ctx context.Context,
		spec operation.Spec,
		work operation.Work,
	) (domain.Operation, error)
}

type Handlers struct {
	git Git
	ops Operations
}

func New(
	git Git,
	ops Operations,
) *Handlers {
	return &Handlers{
		git: git,
		ops: ops,
	}
}

func (h *Handlers) workspaceID(
	ctx *gin.Context,
) string {
	ws, _ := reqscope.Workspace(ctx)
	return ws.ID
}
