package domain

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type DeleteConsent bool

const (
	KeepWorkAtRisk    DeleteConsent = false
	DiscardWorkAtRisk DeleteConsent = true
)

type WorkAtRisk struct {
	WorkspaceID      string `json:"workspaceId"`
	Branch           string `json:"branch"`
	UncommittedFiles int    `json:"uncommittedFiles"`
	UnmergedCommits  int    `json:"unmergedCommits"`
}

func (w WorkAtRisk) Lost() bool {
	return w.UncommittedFiles > 0 || w.UnmergedCommits > 0
}

var ErrWorkAtRisk = problem.New(problem.WorkAtRisk, "delete would destroy work that exists nowhere else")

func WorkAtRisked(
	risks []WorkAtRisk,
) error {
	parts := make([]string, 0, len(risks))
	for _, w := range risks {
		parts = append(parts, fmt.Sprintf("%s (%d uncommitted files, %d unmerged commits)",
			w.Branch, w.UncommittedFiles, w.UnmergedCommits))
	}
	return &problem.Error{
		Code: problem.WorkAtRisk,
		Data: map[string]any{"workAtRisk": risks},
		Err:  fmt.Errorf("%w: %s", ErrWorkAtRisk, strings.Join(parts, "; ")),
	}
}
