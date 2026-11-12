package chat_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func TestAttachmentUpload_ChunksBecomeAReadableAttachment(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	data := pngBytes()

	received, err := f.usecase.BeginAttachmentUpload(f.ctx, chatID, "ab12", "photo.png", int64(len(data)))
	require.NoError(t, err)
	assert.Zero(t, received)
	received, err = f.usecase.AppendAttachmentUpload(f.ctx, chatID, "ab12", 0, bytes.NewReader(data[:9]))
	require.NoError(t, err)
	assert.EqualValues(t, 9, received)
	_, err = f.usecase.AppendAttachmentUpload(f.ctx, chatID, "ab12", 9, bytes.NewReader(data[9:]))
	require.NoError(t, err)

	stored, err := f.usecase.CompleteAttachmentUpload(f.ctx, chatID, "ab12", "photo.png", int64(len(data)))
	require.NoError(t, err)
	assert.Equal(t, "chats/"+chatID+"/attachments/ab12-photo.png", stored.Ref)
	assert.Equal(t, "image/png", stored.ContentType)
	assert.Equal(t, len(data), stored.Size)

	got, _, err := f.usecase.ReadAttachment(f.ctx, chatID, stored.FileName)
	require.NoError(t, err)
	assert.Equal(t, data, got)
}

func TestAttachmentUpload_AbortThenAppendIsNotFound(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	_, err := f.usecase.BeginAttachmentUpload(f.ctx, chatID, "ab12", "a.bin", 4)
	require.NoError(t, err)
	require.NoError(t, f.usecase.AbortAttachmentUpload(f.ctx, chatID, "ab12"))
	_, err = f.usecase.AppendAttachmentUpload(f.ctx, chatID, "ab12", 0, bytes.NewReader([]byte("abcd")))
	assert.Error(t, err)
}

func TestAttachmentUpload_RefusesAnInvalidID(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	_, err := f.usecase.BeginAttachmentUpload(f.ctx, chatID, "not an id!", "a.bin", 4)
	require.ErrorIs(t, err, problem.ErrInvalidArgument)
}

func TestAttachmentUpload_LookupFailuresAreWrapped(t *testing.T) {
	f, cs := newFaultFixture(t)
	chatID, _ := f.spawn(t, "claude")
	cs.failGetChat = fmt.Errorf("boom: get chat")
	_, err := f.usecase.BeginAttachmentUpload(f.ctx, chatID, "ab12", "a.bin", 4)
	require.ErrorContains(t, err, "attachments: chat")

	f2 := newFixture(t)
	chat2, _ := f2.spawn(t, "claude")
	*f2.homeErr = fmt.Errorf("boom: no home")
	err = f2.usecase.AbortAttachmentUpload(f2.ctx, chat2, "ab12")
	require.ErrorContains(t, err, "attachments: home")
}
