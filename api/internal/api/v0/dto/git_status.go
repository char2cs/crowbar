package dto

import (
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type GitFileDTO struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Staged bool   `json:"staged"`
}

type GitStatusDTO struct {
	Branch string       `json:"branch"`
	Ahead  int          `json:"ahead"`
	Behind int          `json:"behind"`
	Files  []GitFileDTO `json:"files"`
}

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
