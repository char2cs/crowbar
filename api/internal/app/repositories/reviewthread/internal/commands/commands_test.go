package commands

import (
	"errors"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestOpen_EmitsOpen(t *testing.T) {
	th := OpenReviewThread{ID: "t1", WsID: "w1", MessageID: "m1", Now: time.Unix(1, 0)}.EmitEvent(nil)
	assert.Equal(t, domain.ReviewThreadStatusOpen, th.Status)
	require.Len(t, th.Messages, 1)
}

func TestResolve_RequiresOpen(t *testing.T) {
	err := ResolveReviewThread{ID: "t1"}.Validate(&domain.ReviewThread{Status: domain.ReviewThreadStatusResolved})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
	th := ResolveReviewThread{ID: "t1"}.EmitEvent(&domain.ReviewThread{Status: domain.ReviewThreadStatusOpen})
	assert.Equal(t, domain.ReviewThreadStatusResolved, th.Status)
}

func TestReopen_RequiresResolved(t *testing.T) {
	err := ReopenReviewThread{ID: "t1"}.Validate(&domain.ReviewThread{Status: domain.ReviewThreadStatusOpen})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
	th := ReopenReviewThread{ID: "t1"}.EmitEvent(&domain.ReviewThread{Status: domain.ReviewThreadStatusResolved})
	assert.Equal(t, domain.ReviewThreadStatusOpen, th.Status)
}

func TestReviewThread_Metadata(t *testing.T) {
	assert.Equal(t, "t1", OpenReviewThread{ID: "t1"}.AggregateID())
	assert.Contains(t, ResolveReviewThread{ID: "t1"}.EventName(), "resolved")
	assert.Contains(t, ReopenReviewThread{ID: "t1"}.EventName(), "reopened")
}

func TestReviewThread_AllMetadata(t *testing.T) {
	open := OpenReviewThread{ID: "t1"}
	assert.Contains(t, open.EventName(), "opened")
	assert.False(t, open.ShouldSnapshot())

	resolve := ResolveReviewThread{ID: "t1"}
	assert.Equal(t, "t1", resolve.AggregateID())
	assert.False(t, resolve.ShouldSnapshot())

	reopen := ReopenReviewThread{ID: "t1"}
	assert.Equal(t, "t1", reopen.AggregateID())
	assert.False(t, reopen.ShouldSnapshot())
}

func TestOpen_Validate_RejectsExisting(t *testing.T) {
	err := OpenReviewThread{ID: "t1", WsID: "w1"}.Validate(&domain.ReviewThread{ID: "t1"})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestOpen_Validate_RejectsMissingIDs(t *testing.T) {
	err := OpenReviewThread{ID: "t1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestOpen_Validate_AcceptsValidNew(t *testing.T) {
	err := OpenReviewThread{ID: "t1", WsID: "w1", MessageID: "m1"}.Validate(nil)
	assert.NoError(t, err)
}

func TestResolve_Validate_RejectsNil(t *testing.T) {
	err := ResolveReviewThread{ID: "t1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestResolve_Validate_AcceptsOpen(t *testing.T) {
	err := ResolveReviewThread{ID: "t1"}.Validate(&domain.ReviewThread{Status: domain.ReviewThreadStatusOpen})
	assert.NoError(t, err)
}

func TestReopen_Validate_RejectsNil(t *testing.T) {
	err := ReopenReviewThread{ID: "t1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestReopen_Validate_AcceptsResolved(t *testing.T) {
	err := ReopenReviewThread{ID: "t1"}.Validate(&domain.ReviewThread{Status: domain.ReviewThreadStatusResolved})
	assert.NoError(t, err)
}

func TestOpenReviewThread_SeedsAnchorAndFirstMessage(t *testing.T) {
	now := time.Unix(1, 0)
	cmd := OpenReviewThread{
		ID: "t1", WsID: "w1", FilePath: "a.go", LineNumber: 12,
		Side: domain.ReviewSideRight, MessageID: "m1", Body: "hi", Now: now,
	}
	th := cmd.EmitEvent(nil)
	assert.Equal(t, "a.go", th.FilePath)
	assert.Equal(t, 12, th.LineNumber)
	assert.Equal(t, domain.ReviewSideRight, th.Side)
	require.Len(t, th.Messages, 1)
	assert.Equal(t, "hi", th.Messages[0].Body)
	assert.Equal(t, domain.ReviewThreadStatusOpen, th.Status)
}

func TestReplyReviewThread_AppendsMessage(t *testing.T) {
	now := time.Unix(2, 0)
	cur := &domain.ReviewThread{
		ID: "t1", Status: domain.ReviewThreadStatusOpen,
		Messages: []domain.ReviewMessage{{ID: "m1", Body: "first"}},
	}
	th := ReplyReviewThread{ID: "t1", MessageID: "m2", Body: "second", Now: now}.EmitEvent(cur)
	require.Len(t, th.Messages, 2)
	assert.Equal(t, "second", th.Messages[1].Body)
}

func TestReplyReviewThread_Validate_RejectsMissing(t *testing.T) {
	assert.True(t, errors.Is(ReplyReviewThread{ID: "t1"}.Validate(nil), asynxModels.ErrValidation))
}

func TestReplyReviewThread_Validate_RejectsMissingMessageID(t *testing.T) {
	assert.True(t, errors.Is(ReplyReviewThread{ID: "t1"}.Validate(&domain.ReviewThread{ID: "t1"}), asynxModels.ErrValidation))
}

func TestReplyReviewThread_Validate_Accepts(t *testing.T) {
	assert.NoError(t, ReplyReviewThread{ID: "t1", MessageID: "m1"}.Validate(&domain.ReviewThread{ID: "t1"}))
}

func TestCommands_Metadata(t *testing.T) {
	open := OpenReviewThread{ID: "t1"}
	assert.Equal(t, "t1", open.AggregateID())
	assert.Contains(t, open.EventName(), "opened")
	assert.False(t, open.ShouldSnapshot())

	reply := ReplyReviewThread{ID: "t1"}
	assert.Equal(t, "t1", reply.AggregateID())
	assert.Contains(t, reply.EventName(), "replied")
	assert.False(t, reply.ShouldSnapshot())

	resolve := ResolveReviewThread{ID: "t1"}
	assert.Equal(t, "t1", resolve.AggregateID())
	assert.Contains(t, resolve.EventName(), "resolved")
	assert.False(t, resolve.ShouldSnapshot())

	reopen := ReopenReviewThread{ID: "t1"}
	assert.Equal(t, "t1", reopen.AggregateID())
	assert.Contains(t, reopen.EventName(), "reopened")
	assert.False(t, reopen.ShouldSnapshot())
}
