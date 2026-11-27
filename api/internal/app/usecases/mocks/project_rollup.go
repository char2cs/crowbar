package mocks

import (
	"context"
	"time"
)

type ProjectRollup struct {
	TouchedRepoID string
	Touched       bool
}

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
