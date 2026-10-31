package v0

import (
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// threadStream publishes a review thread a REST write produced on its
// workspace's ws: scope; a tombstone says the thread is gone.
type threadStream struct{ hub *appstream.Hub }

func (t threadStream) Push(d dto.ThreadDTO) {
	scope := stream.Scope{Kind: stream.KindWs, Key: d.WorkspaceID}
	if d.Deleted {
		t.hub.Gone(scope, stream.KindReviewThread, d.ID)
		return
	}
	t.hub.Entity(scope, stream.KindReviewThread, d.ID, d)
}
