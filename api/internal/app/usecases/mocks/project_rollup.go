package mocks

import (
	"context"
	"time"
)

// ProjectRollup is a fake of the project lastActivity roll-up surface. It
// records the last repoID it was asked to touch.
type ProjectRollup struct {
	TouchedRepoID string
	Touched       bool
}

// NewProjectRollup returns an empty ProjectRollup.
func NewProjectRollup() *ProjectRollup {
	return &ProjectRollup{}
}

func (r *ProjectRollup) TouchProjectActivity(
	ctx context.Context,
	repoID string,
	now time.Time,
) {
	r.TouchedRepoID = repoID
	r.Touched = true
}
