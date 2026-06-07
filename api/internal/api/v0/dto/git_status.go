package dto

import (
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// GitFileDTO is the wire shape of a single changed file in a workspace's
// working-tree status (04 §3): its workspace-relative path, its per-file status,
// and whether the change is staged.
type GitFileDTO struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Staged bool   `json:"staged"`
}

// GitStatusDTO is the wire shape of a workspace's full working-tree status
// (04 §3): the current branch, its ahead/behind counts against the upstream,
// and the list of changed files.
type GitStatusDTO struct {
	Branch string       `json:"branch"`
	Ahead  int          `json:"ahead"`
	Behind int          `json:"behind"`
	Files  []GitFileDTO `json:"files"`
}

// GitStatusDTOFrom converts a domain GitStatus into its wire DTO. The files
// slice is always non-nil so the envelope's data serialises with files: []
// rather than null.
func GitStatusDTOFrom(
	status gitdomain.GitStatus,
) GitStatusDTO {
	files := make([]GitFileDTO, 0, len(status.Files))
	for _, f := range status.Files {
		files = append(files, gitFileDTOFrom(f))
	}
	return GitStatusDTO{
		Branch: status.Branch,
		Ahead:  status.Ahead,
		Behind: status.Behind,
		Files:  files,
	}
}

func gitFileDTOFrom(
	file gitdomain.GitFile,
) GitFileDTO {
	return GitFileDTO{
		Path:   file.Path,
		Status: string(file.Status),
		Staged: file.Staged,
	}
}
