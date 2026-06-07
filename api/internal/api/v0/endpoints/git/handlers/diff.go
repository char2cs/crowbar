package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// Diff handles GET /v0/workspaces/:wsId/git/diff. With a commit query parameter
// it returns that commit's MultiFileDiff; otherwise it returns the working-tree
// diff as a FileDiffDTO list. The staged query parameter (default false)
// selects the staged diff over the unstaged one, and an optional path parameter
// scopes either diff to a single file. A non-boolean staged value is rejected
// with 400.
func (h *Handlers) Diff(
	c *gin.Context,
) {
	commit := c.Query("commit")
	if commit != "" {
		h.commitDiff(c, commit)
		return
	}
	h.workingTreeDiff(c)
}

func (h *Handlers) commitDiff(
	c *gin.Context,
	commit string,
) {
	diff, err := h.git.CommitDiff(c.Request.Context(), c.Param("wsId"), commit)
	if err != nil {
		code, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, code, msg)
		return
	}
	scoped := scopeMultiFileDiff(diff, c.Query("path"))
	libs.WriteQueryOK(c, dto.MultiFileDiffDTOFrom(scoped))
}

func (h *Handlers) workingTreeDiff(
	c *gin.Context,
) {
	staged, ok := parseBool(c.Query("staged"), false)
	if !ok {
		libs.WriteErr(c, http.StatusBadRequest, "invalid staged")
		return
	}
	diffs, err := h.git.Diff(c.Request.Context(), c.Param("wsId"), staged)
	if err != nil {
		code, msg := libs.StatusAndMessage(err)
		libs.WriteErr(c, code, msg)
		return
	}
	libs.WriteQueryOK(c, dto.FileDiffDTOList(filterByPath(diffs, c.Query("path"))))
}

func parseBool(
	raw string,
	fallback bool,
) (bool, bool) {
	if raw == "" {
		return fallback, true
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, false
	}
	return v, true
}

func filterByPath(
	diffs []gitdomain.FileDiff,
	path string,
) []gitdomain.FileDiff {
	if path == "" {
		return diffs
	}
	out := make([]gitdomain.FileDiff, 0, len(diffs))
	for _, diff := range diffs {
		if diff.FilePath == path {
			out = append(out, diff)
		}
	}
	return out
}

// scopeMultiFileDiff narrows a commit's MultiFileDiff to the single file at path
// and recomputes the aggregate totals so they stay consistent with the retained
// file. An empty path returns the diff unchanged.
func scopeMultiFileDiff(
	diff gitdomain.MultiFileDiff,
	path string,
) gitdomain.MultiFileDiff {
	if path == "" {
		return diff
	}
	files := filterByPath(diff.Files, path)
	diff.Files = files
	diff.TotalFiles = len(files)
	diff.TotalAdditions = 0
	diff.TotalDeletions = 0
	for _, f := range files {
		diff.TotalAdditions += f.Additions
		diff.TotalDeletions += f.Deletions
	}
	return diff
}
