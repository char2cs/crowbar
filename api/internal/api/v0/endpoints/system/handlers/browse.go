package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

// BrowseEntry is one child of a listed directory.
type BrowseEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

// Browse serves GET /v0/system/browse?path=<absolute path, default home> for
// the remote file picker. Not workspace-sandboxed: it is how a client picks
// where to create one. Files are listed too; the client shows them inert.
func Browse(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			libs.WriteErr(c, http.StatusInternalServerError, "could not resolve a home directory")
			return
		}
		path = home
	}
	if !filepath.IsAbs(path) {
		libs.WriteErr(c, http.StatusBadRequest, "path must be absolute")
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "cannot read path: "+err.Error())
		return
	}
	if !info.IsDir() {
		libs.WriteErr(c, http.StatusBadRequest, "path is not a directory")
		return
	}

	dirEntries, err := os.ReadDir(path)
	if err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "cannot read directory: "+err.Error())
		return
	}

	entries := make([]BrowseEntry, 0, len(dirEntries))
	for _, e := range dirEntries {
		childPath := filepath.Join(path, e.Name())
		isDir := e.IsDir()
		// For symlinks, resolve the actual type instead of trusting IsDir().
		// os.DirEntry.IsDir() returns false for symlinks even when they point
		// to directories, so we need to stat the full path to get the truth.
		if e.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(childPath); err == nil {
				isDir = info.IsDir()
			}
		}
		entries = append(entries, BrowseEntry{
			Name:  e.Name(),
			Path:  childPath,
			IsDir: isDir,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})

	libs.WriteQueryOK(c, entries)
}
