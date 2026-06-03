package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
)

// Handlers holds git HTTP handlers.
type Handlers struct {
	svc usecases.GitUsecase
}

// New returns a Handlers wired to svc.
func New(
	svc usecases.GitUsecase,
) *Handlers {
	return &Handlers{svc: svc}
}

// DiffFile holds per-file diff statistics.
type DiffFile struct {
	Name    string `json:"name"`
	Hunks   int    `json:"hunks"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
}

// GitDiff holds diff statistics per changed file.
type GitDiff struct {
	Files []DiffFile `json:"files"`
}

// Log handles GET /tasks/:id/git/log.
func (h *Handlers) Log(
	c *gin.Context,
) {
	id := c.Param("id")
	commits, err := h.svc.Log(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			libs.WriteErr(c, http.StatusNotFound, "not found", id)
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), id)
		return
	}
	libs.WriteQueryOK(c, commits)
}

// Diff handles GET /tasks/:id/git/diff.
func (h *Handlers) Diff(
	c *gin.Context,
) {
	id := c.Param("id")
	rawDiff, err := h.svc.Diff(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			libs.WriteErr(c, http.StatusNotFound, "not found", id)
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), id)
		return
	}
	fileStats := parseUnifiedDiff(rawDiff)
	var files []DiffFile
	for _, stat := range fileStats {
		files = append(files, stat)
	}
	if files == nil {
		files = []DiffFile{}
	}
	libs.WriteQueryOK(c, GitDiff{Files: files})
}

// ListFiles handles GET /tasks/:id/files.
func (h *Handlers) ListFiles(
	c *gin.Context,
) {
	id := c.Param("id")
	files, err := h.svc.ListFiles(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			libs.WriteErr(c, http.StatusNotFound, "not found", id)
			return
		}
		libs.WriteErr(c, http.StatusInternalServerError, err.Error(), id)
		return
	}
	libs.WriteQueryOK(c, files)
}

func parseUnifiedDiff(diff string) map[string]DiffFile {
	result := make(map[string]DiffFile)
	var currentFile string
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ b/"):
			currentFile = strings.TrimPrefix(line, "+++ b/")
			if _, exists := result[currentFile]; !exists {
				result[currentFile] = DiffFile{Name: currentFile}
			}
		case strings.HasPrefix(line, "@@ "):
			if currentFile == "" {
				continue
			}
			stat := result[currentFile]
			stat.Hunks++
			result[currentFile] = stat
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			if currentFile == "" {
				continue
			}
			stat := result[currentFile]
			stat.Added++
			result[currentFile] = stat
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			if currentFile == "" {
				continue
			}
			stat := result[currentFile]
			stat.Removed++
			result[currentFile] = stat
		}
	}
	return result
}
