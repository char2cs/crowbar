package domain

import (
	"errors"
	"fmt"
	"strings"
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

// ErrWorkAtRisk is the sentinel a WorkAtRiskError unwraps to.
var ErrWorkAtRisk = errors.New("delete would destroy work that exists nowhere else")

// WorkAtRiskError refuses a delete made without consent, naming every
// workspace whose work it would have destroyed. Nothing was touched.
type WorkAtRiskError struct {
	Workspaces []WorkAtRisk
}

func (e *WorkAtRiskError) Error() string {
	parts := make([]string, 0, len(e.Workspaces))
	for _, w := range e.Workspaces {
		parts = append(parts, fmt.Sprintf("%s (%d uncommitted files, %d unmerged commits)",
			w.Branch, w.UncommittedFiles, w.UnmergedCommits))
	}
	return ErrWorkAtRisk.Error() + ": " + strings.Join(parts, "; ")
}

func (e *WorkAtRiskError) Unwrap() error {
	return ErrWorkAtRisk
}
