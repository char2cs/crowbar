package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type spyThreadHub struct {
	pushed []dto.ThreadDTO
}

func (s *spyThreadHub) BroadcastThread(
	t dto.ThreadDTO,
) {
	s.pushed = append(s.pushed, t)
}

// TestAgentThreadBroadcast_StampsTheOwningScopeOntoTheFrame is the other half of
// Task 12's fan-out: agenttools proves post_review_comment CALLS the seam, and this
// proves the seam produces a frame the /threads stream can actually route.
//
// The stream filters on projectId, repoId AND wsId (api/v0.threadStream). The
// domain aggregate carries only WsID, so a frame assembled without the two ids the
// caller's workspace supplies would match no subscriber and the comment would still
// be invisible — the exact failure this seam exists to fix.
func TestAgentThreadBroadcast_StampsTheOwningScopeOntoTheFrame(t *testing.T) {
	spy := &spyThreadHub{}
	broadcast := agentThreadBroadcast(spy)

	broadcast(domain.ReviewThread{
		ID:        "t1",
		WsID:      "w1",
		FilePath:  "src/auth.go",
		StartLine: 42,
		EndLine:   44,
		Side:      domain.ReviewSideRight,
		Status:    domain.ReviewThreadStatusOpen,
		Messages: []domain.ReviewMessage{{
			ID: "m1", Author: "claude", IsAgent: true, Body: "This leaks the token.",
			CreatedAt: time.Unix(1, 0).UTC(),
		}},
	}, "p1", "r1")

	require.Len(t, spy.pushed, 1)
	got := spy.pushed[0]
	require.Equal(t, "t1", got.ID)
	require.Equal(t, "w1", got.WorkspaceID)
	require.Equal(t, "p1", got.ProjectID)
	require.Equal(t, "r1", got.RepoID)
	require.Equal(t, "src/auth.go", got.FilePath)
	require.Equal(t, 42, got.StartLine)
	require.Equal(t, 44, got.EndLine)
	require.Equal(t, "This leaks the token.", got.Body)
	require.True(t, got.IsAgent, "the frame must tell the UI this came from an agent")
}
