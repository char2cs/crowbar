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

func TestCreateChat_EmitsIdle(t *testing.T) {
	chat := CreateChat{ID: "c1", WsID: "w1", Now: time.Unix(1, 0)}.EmitEvent(nil)
	assert.Equal(t, domain.ChatStatusIdle, chat.Status)
}

func TestCreateChat_Validate_RejectsExisting(t *testing.T) {
	err := CreateChat{ID: "c1", WsID: "w1"}.Validate(&domain.Chat{ID: "c1"})
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestChat_AllMetadata(t *testing.T) {
	create := CreateChat{ID: "c1"}
	assert.Equal(t, "c1", create.AggregateID())
	assert.Contains(t, create.EventName(), "chat.created")
	assert.False(t, create.ShouldSnapshot())
}

func TestCreateChat_Validate_RejectsMissingIDs(t *testing.T) {
	err := CreateChat{ID: "c1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestCreateChat_Validate_AcceptsValidNew(t *testing.T) {
	err := CreateChat{ID: "c1", WsID: "w1"}.Validate(nil)
	assert.NoError(t, err)
}

func TestCreateChat_SeedsTitleAndType(t *testing.T) {
	now := time.Unix(1, 0)
	cmd := CreateChat{ID: "c1", WsID: "w1", Title: "Hello", Now: now}
	c := cmd.EmitEvent(nil)
	assert.Equal(t, "Hello", c.Title)
	assert.Equal(t, domain.ChatTypeChat, c.Type)
	assert.Equal(t, domain.ChatStatusIdle, c.Status)
}

func TestForkChat_CopiesParentTitleWithParentID(t *testing.T) {
	now := time.Unix(2, 0)
	cmd := ForkChat{ID: "c2", WsID: "w1", ParentID: "c1", Title: "Hello (fork)", Now: now}
	c := cmd.EmitEvent(nil)
	assert.Equal(t, "c1", c.ParentID)
	assert.Equal(t, "Hello (fork)", c.Title)
	assert.Equal(t, domain.ChatStatusIdle, c.Status)
}

func TestForkChat_Validate_RejectsExistingAndMissing(t *testing.T) {
	assert.True(t, errors.Is(ForkChat{ID: "c2", WsID: "w1", ParentID: "c1"}.Validate(&domain.Chat{ID: "c2"}), asynxModels.ErrValidation))
	assert.True(t, errors.Is(ForkChat{ID: "c2"}.Validate(nil), asynxModels.ErrValidation))
}

func TestRenameChat_UpdatesTitle(t *testing.T) {
	cur := &domain.Chat{ID: "c1", Title: "old"}
	c := RenameChat{ID: "c1", Title: "new"}.EmitEvent(cur)
	assert.Equal(t, "new", c.Title)
}

func TestRenameChat_Validate_RejectsMissing(t *testing.T) {
	assert.True(t, errors.Is(RenameChat{ID: "c1"}.Validate(nil), asynxModels.ErrValidation))
}

func TestDeleteChat_SetsDeletedAt(t *testing.T) {
	now := time.Unix(5, 0)
	cur := &domain.Chat{ID: "c1", Status: domain.ChatStatusIdle}
	c := DeleteChat{ID: "c1", Now: now}.EmitEvent(cur)
	require.NotNil(t, c.DeletedAt)
	assert.Equal(t, now, *c.DeletedAt)
}

func TestDeleteChat_Idempotent(t *testing.T) {
	first := time.Unix(5, 0)
	cur := &domain.Chat{ID: "c1", DeletedAt: &first}
	c := DeleteChat{ID: "c1", Now: time.Unix(9, 0)}.EmitEvent(cur)
	require.NotNil(t, c.DeletedAt)
	assert.Equal(t, first, *c.DeletedAt)
}

func TestCommands_Metadata(t *testing.T) {
	create := CreateChat{ID: "c1"}
	assert.Equal(t, "c1", create.AggregateID())
	assert.Contains(t, create.EventName(), "chat.created")
	assert.False(t, create.ShouldSnapshot())

	fork := ForkChat{ID: "c2"}
	assert.Equal(t, "c2", fork.AggregateID())
	assert.Contains(t, fork.EventName(), "chat.forked")
	assert.False(t, fork.ShouldSnapshot())

	rename := RenameChat{ID: "c3"}
	assert.Equal(t, "c3", rename.AggregateID())
	assert.Contains(t, rename.EventName(), "chat.renamed")
	assert.False(t, rename.ShouldSnapshot())

	del := DeleteChat{ID: "c4"}
	assert.Equal(t, "c4", del.AggregateID())
	assert.Contains(t, del.EventName(), "chat.deleted")
	assert.False(t, del.ShouldSnapshot())
}

func TestForkChat_Validate_AcceptsValidNew(t *testing.T) {
	err := ForkChat{ID: "c2", WsID: "w1", ParentID: "c1"}.Validate(nil)
	assert.NoError(t, err)
}

func TestRenameChat_Validate_AcceptsExisting(t *testing.T) {
	err := RenameChat{ID: "c1"}.Validate(&domain.Chat{ID: "c1"})
	assert.NoError(t, err)
}

func TestDeleteChat_Validate_RejectsMissing(t *testing.T) {
	err := DeleteChat{ID: "c1"}.Validate(nil)
	assert.True(t, errors.Is(err, asynxModels.ErrValidation))
}

func TestDeleteChat_Validate_AcceptsExisting(t *testing.T) {
	err := DeleteChat{ID: "c1"}.Validate(&domain.Chat{ID: "c1"})
	assert.NoError(t, err)
}
