package dto

import (
	"github.com/char2cs/crowbar/api/internal/domain"
)

// FileNodeDTO is the wire shape of a file-tree entry (05 §2): the display name,
// the workspace-relative path, the file/directory discriminator, the lazily
// loaded children, and an optional git decoration. Children are omitted when a
// node has not been expanded, and gitStatus is omitted for clean files.
type FileNodeDTO struct {
	Name      string                    `json:"name"`
	Path      string                    `json:"path"`
	Type      domain.FileNodeType       `json:"type"`
	Children  []FileNodeDTO             `json:"children,omitempty"`
	GitStatus *domain.FileNodeGitStatus `json:"gitStatus,omitempty"`
}

// FileNodeDTOFrom converts a domain FileNode into its wire DTO, recursing into
// any loaded children.
func FileNodeDTOFrom(
	node domain.FileNode,
) FileNodeDTO {
	return FileNodeDTO{
		Name:      node.Name,
		Path:      node.Path,
		Type:      node.Type,
		Children:  FileNodeDTOList(node.Children),
		GitStatus: node.GitStatus,
	}
}

// FileNodeDTOList converts a slice of domain FileNodes into wire DTOs. It
// returns nil for an empty input so the children field stays omitted on leaf
// nodes; the top-level tree handler substitutes a non-nil empty slice.
func FileNodeDTOList(
	nodes []domain.FileNode,
) []FileNodeDTO {
	if len(nodes) == 0 {
		return nil
	}
	dtos := make([]FileNodeDTO, 0, len(nodes))
	for _, node := range nodes {
		dtos = append(dtos, FileNodeDTOFrom(node))
	}
	return dtos
}

// FileContentDTO is the wire shape of a file-content read (05 §3): the file body
// and its encoding. Encoding is "base64" for binary files and omitted for UTF-8
// text.
type FileContentDTO struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding,omitempty"`
}

// FileContentDTOFrom converts a domain FileContent into its wire DTO.
func FileContentDTOFrom(
	content domain.FileContent,
) FileContentDTO {
	return FileContentDTO{
		Content:  content.Content,
		Encoding: content.Encoding,
	}
}
