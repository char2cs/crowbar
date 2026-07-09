package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

var errBoom = errors.New("boom")

type errGit struct{}

func (errGit) Status(_ context.Context, _ string) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{}, errBoom
}

func (errGit) Diff(_ context.Context, _ string, _ bool) ([]gitdomain.FileDiff, error) {
	return nil, errBoom
}

func (errGit) Log(_ context.Context, _ string, _, _ int) ([]gitdomain.Commit, error) {
	return nil, errBoom
}

func (errGit) Blame(_ context.Context, _, _ string) ([]gitdomain.BlameEntry, error) {
	return nil, errBoom
}

func (errGit) Branches(_ context.Context, _ string) ([]gitdomain.Branch, error) {
	return nil, errBoom
}

func (errGit) Stashes(_ context.Context, _ string) ([]gitdomain.Stash, error) {
	return nil, errBoom
}

func (errGit) ConflictedFiles(_ context.Context, _ string) ([]string, error) {
	return nil, errBoom
}

func (errGit) ConflictHunks(_ context.Context, _, _ string) ([]gitdomain.ConflictHunk, error) {
	return nil, errBoom
}

func (errGit) CommitDiff(_ context.Context, _, _ string) (gitdomain.MultiFileDiff, error) {
	return gitdomain.MultiFileDiff{}, errBoom
}

func (errGit) StageFile(_ context.Context, _, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) StageHunk(_ context.Context, _, _, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) UnstageFile(_ context.Context, _, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) UnstageHunk(_ context.Context, _, _, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) Discard(_ context.Context, _, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) Commit(_ context.Context, _, _, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) Push(_ context.Context, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) Fetch(_ context.Context, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) Pull(_ context.Context, _, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) CreateBranch(_ context.Context, _, _, _ string, _ bool, _ time.Time) error {
	return errBoom
}

func (errGit) RenameBranch(_ context.Context, _, _, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) DeleteBranch(_ context.Context, _, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) SwitchBranch(_ context.Context, _, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) StashPush(_ context.Context, _, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) StashApply(_ context.Context, _, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) StashPop(_ context.Context, _, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) StashDrop(_ context.Context, _, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) Reset(_ context.Context, _, _, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) Merge(_ context.Context, _, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) Rebase(_ context.Context, _, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) ResolveHunk(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ gitdomain.ConflictResolution,
	_ string,
	_ time.Time,
) error {
	return errBoom
}

func (errGit) OperationContinue(_ context.Context, _ string, _ time.Time) error {
	return errBoom
}

func (errGit) OperationAbort(_ context.Context, _ string, _ time.Time) error {
	return errBoom
}

func TestGitReadHandlers_Errors(
	t *testing.T,
) {
	r := newRouterWith(errGit{})

	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodGet, ws+"/status", nil).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodGet, ws+"/diff", nil).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodGet, ws+"/log", nil).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodGet, ws+"/blame?path=main.go", nil).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodGet, ws+"/branches", nil).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodGet, ws+"/stashes", nil).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodGet, ws+"/conflicts", nil).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodGet, ws+"/conflict-hunks?path=main.go", nil).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodGet, ws+"/commit-diff?sha=abc", nil).Code)
}

func TestGitWriteHandlers_Errors(
	t *testing.T,
) {
	r := newRouterWith(errGit{})

	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPost, ws+"/stage",
		map[string]any{"paths": []string{"a.go"}}).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPost, ws+"/stage-hunk",
		map[string]any{"path": "a.go", "hunkId": "h1"}).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPost, ws+"/unstage",
		map[string]any{"paths": []string{"a.go"}}).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPost, ws+"/unstage-hunk",
		map[string]any{"path": "a.go", "hunkId": "h1"}).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPost, ws+"/discard",
		map[string]any{"paths": []string{"a.go"}}).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPost, ws+"/commit",
		map[string]any{"subject": "feat: x"}).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPost, ws+"/branches",
		map[string]any{"name": "feat/x"}).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPatch, ws+"/branches",
		map[string]any{"from": "old", "to": "new"}).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodDelete, ws+"/branches?name=feat/x", nil).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPost, ws+"/switch",
		map[string]any{"branch": "main"}).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPost, ws+"/stash", nil).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPost, ws+"/stash-apply",
		map[string]any{"index": 0}).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPost, ws+"/stash-pop",
		map[string]any{"index": 0}).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodDelete, ws+"/stash?index=0", nil).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPost, ws+"/reset",
		map[string]any{"mode": "soft"}).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPost, ws+"/resolve-hunk",
		map[string]any{"path": "a.go", "hunkIndex": 0, "choice": "ours"}).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPost, ws+"/operation/continue", nil).Code)
	assert.Equal(t, http.StatusInternalServerError, do(r, http.MethodPost, ws+"/operation/abort", nil).Code)
}
