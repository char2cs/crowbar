package tree

import (
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// Evict empties projectID's held tree, as a write that may have committed
// unseen does.
func (r *Repository) Evict(projectID string) { r.evict(projectID) }

// Begin and End are send's halves, so a test can end writes in an order the
// scheduler rarely produces.
func (r *Repository) Begin(projectID string) int64 { return r.begin(projectID) }

func (r *Repository) End(projectID string, epoch int64, evt asynxModels.Event[tree.Tree], err error) {
	r.end(projectID, epoch, evt, err)
}
