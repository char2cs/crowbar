package domain

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// DeleteConsent says whether a delete may destroy work that exists nowhere
// else. Only a client that has shown the user the WorkAtRisk list sends
// DiscardWorkAtRisk; everything else (boot resume, merge fold, rollback) keeps.
type DeleteConsent bool

const (
	KeepWorkAtRisk    DeleteConsent = false
	DiscardWorkAtRisk DeleteConsent = true
)

// WorkAtRisk is what tearing one workspace down would destroy that git cannot
// bring back: uncommitted paths in a worktree about to be force-removed, and
// commits no other branch, remote-tracking branch or tag reaches.
type WorkAtRisk struct {
	WorkspaceID      string `json:"workspaceId"`
	Branch           string `json:"branch"`
	UncommittedFiles int    `json:"uncommittedFiles"`
	UnmergedCommits  int    `json:"unmergedCommits"`
}

// Lost reports whether anything would be lost at all.
func (w WorkAtRisk) Lost() bool {
	return w.UncommittedFiles > 0 || w.UnmergedCommits > 0
}

// ErrWorkAtRisk is the refusal of a delete made without consent.
var ErrWorkAtRisk = problem.New(problem.WorkAtRisk, "delete would destroy work that exists nowhere else")

// WorkAtRisked refuses a delete made without consent, naming every workspace
// whose work it would have destroyed; data.workAtRisk is what the client asks
// the user about before it retries with consent. Nothing was touched.
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
