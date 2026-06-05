// Package tree provides a lazy one-level file tree walker with git decorations (05 §2).
package tree

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// StatusProvider resolves git status for a repo path. Implemented by the git
// engine's status package; injected to avoid a circular import.
type StatusProvider interface {
	GitStatus(
		repoPath string,
	) (domain.GitStatus, error)
}

// List returns one level of the file tree rooted at dirPath, with git
// decorations from provider. dirPath is relative to repoPath (empty = root).
func List(
	repoPath string,
	dirPath string,
	provider StatusProvider,
) ([]domain.FileNode, error) {
	full := filepath.Join(repoPath, dirPath)
	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, fmt.Errorf("tree: readdir %s: %w", dirPath, err)
	}

	gitFiles := buildStatusMap(repoPath, provider)

	nodes := make([]domain.FileNode, 0, len(entries))
	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}
		node := buildNode(repoPath, dirPath, e, gitFiles)
		nodes = append(nodes, node)
	}

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Type != nodes[j].Type {
			return nodes[i].Type == domain.FileNodeTypeDirectory
		}
		return nodes[i].Name < nodes[j].Name
	})

	return nodes, nil
}

func buildStatusMap(
	repoPath string,
	provider StatusProvider,
) map[string]domain.GitFileStatus {
	if provider == nil {
		return nil
	}
	status, err := provider.GitStatus(repoPath)
	if err != nil {
		return nil
	}
	m := make(map[string]domain.GitFileStatus, len(status.Files))
	for _, f := range status.Files {
		m[f.Path] = f.Status
	}
	return m
}

func buildNode(
	repoPath string,
	dirPath string,
	e os.DirEntry,
	gitFiles map[string]domain.GitFileStatus,
) domain.FileNode {
	relPath := filepath.Join(dirPath, e.Name())
	nodeType := domain.FileNodeTypeFile
	if e.IsDir() {
		nodeType = domain.FileNodeTypeDirectory
	}
	node := domain.FileNode{
		Name: e.Name(),
		Path: relPath,
		Type: nodeType,
	}
	if gs, ok := gitFiles[relPath]; ok {
		dec := gitDecoration(gs)
		if dec != "" {
			node.GitStatus = &dec
		}
	}
	return node
}

func gitDecoration(
	status domain.GitFileStatus,
) domain.FileNodeGitStatus {
	switch status {
	case domain.GitFileStatusModified:
		return domain.FileNodeGitStatusModified
	case domain.GitFileStatusAdded:
		return domain.FileNodeGitStatusAdded
	case domain.GitFileStatusDeleted:
		return domain.FileNodeGitStatusDeleted
	case domain.GitFileStatusUntracked:
		return domain.FileNodeGitStatusUntracked
	case domain.GitFileStatusRenamed:
		return domain.FileNodeGitStatusRenamed
	case domain.GitFileStatusConflicted:
		return domain.FileNodeGitStatusModified
	}
	return ""
}
