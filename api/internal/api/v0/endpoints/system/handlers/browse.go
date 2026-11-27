package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

type BrowseEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

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
