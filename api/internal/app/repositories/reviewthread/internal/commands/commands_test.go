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

func threadWith(messages ...domain.ReviewMessage) *domain.ReviewThread {
	return &domain.ReviewThread{ID: "t1", Status: domain.ReviewThreadStatusOpen, Messages: messages}
}

func TestEditReviewMessage_RewritesBody(t *testing.T) {
	cur := threadWith(
		domain.ReviewMessage{ID: "m1", Body: "root"},
		domain.ReviewMessage{ID: "m2", Body: "old reply"},
	)
	th := EditReviewMessage{ID: "t1", MessageID: "m2", Body: "new reply"}.EmitEvent(cur)
	require.Len(t, th.Messages, 2)
	assert.Equal(t, "new reply", th.Messages[1].Body)
	// Original aggregate is not mutated in place.
	assert.Equal(t, "old reply", cur.Messages[1].Body)
}

func TestEditReviewMessage_CanEditRoot(t *testing.T) {
	cur := threadWith(domain.ReviewMessage{ID: "m1", Body: "root"})
	th := EditReviewMessage{ID: "t1", MessageID: "m1", Body: "edited root"}.EmitEvent(cur)
	assert.Equal(t, "edited root", th.Messages[0].Body)
}

func TestEditReviewMessage_Validate(t *testing.T) {
	cur := threadWith(domain.ReviewMessage{ID: "m1", Body: "root"})
	assert.True(t, errors.Is(EditReviewMessage{ID: "t1", MessageID: "m1", Body: "x"}.Validate(nil), asynxModels.ErrValidation))
	assert.True(t, errors.Is(EditReviewMessage{ID: "t1", Body: "x"}.Validate(cur), asynxModels.ErrValidation))
	assert.True(t, errors.Is(EditReviewMessage{ID: "t1", MessageID: "m1"}.Validate(cur), asynxModels.ErrValidation))
	assert.True(t, errors.Is(EditReviewMessage{ID: "t1", MessageID: "nope", Body: "x"}.Validate(cur), asynxModels.ErrValidation))
	assert.NoError(t, EditReviewMessage{ID: "t1", MessageID: "m1", Body: "x"}.Validate(cur))
}

func TestDeleteReviewMessage_RemovesReply(t *testing.T) {
	cur := threadWith(
		domain.ReviewMessage{ID: "m1", Body: "root"},
		domain.ReviewMessage{ID: "m2", Body: "reply"},
	)
	th := DeleteReviewMessage{ID: "t1", MessageID: "m2"}.EmitEvent(cur)
	require.Len(t, th.Messages, 1)
	assert.Equal(t, "m1", th.Messages[0].ID)
	// Original aggregate is not mutated in place.
	require.Len(t, cur.Messages, 2)
}

func TestDeleteReviewMessage_Validate_RejectsRoot(t *testing.T) {
	cur := threadWith(
		domain.ReviewMessage{ID: "m1", Body: "root"},
		domain.ReviewMessage{ID: "m2", Body: "reply"},
	)
	// Deleting the root (Messages[0]) is rejected — use DeleteThread instead.
	assert.True(t, errors.Is(DeleteReviewMessage{ID: "t1", MessageID: "m1"}.Validate(cur), asynxModels.ErrValidation))
}

func TestDeleteReviewMessage_Validate(t *testing.T) {
	cur := threadWith(
		domain.ReviewMessage{ID: "m1", Body: "root"},
		domain.ReviewMessage{ID: "m2", Body: "reply"},
	)
	assert.True(t, errors.Is(DeleteReviewMessage{ID: "t1", MessageID: "m2"}.Validate(nil), asynxModels.ErrValidation))
	assert.True(t, errors.Is(DeleteReviewMessage{ID: "t1"}.Validate(cur), asynxModels.ErrValidation))
	assert.True(t, errors.Is(DeleteReviewMessage{ID: "t1", MessageID: "nope"}.Validate(cur), asynxModels.ErrValidation))
	assert.NoError(t, DeleteReviewMessage{ID: "t1", MessageID: "m2"}.Validate(cur))
}

func TestEditDeleteMessage_Metadata(t *testing.T) {
	edit := EditReviewMessage{ID: "t1"}
	assert.Equal(t, "t1", edit.AggregateID())
	assert.Contains(t, edit.EventName(), "message_edited")
	assert.False(t, edit.ShouldSnapshot())

	del := DeleteReviewMessage{ID: "t1"}
	assert.Equal(t, "t1", del.AggregateID())
	assert.Contains(t, del.EventName(), "message_deleted")
	assert.False(t, del.ShouldSnapshot())
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
