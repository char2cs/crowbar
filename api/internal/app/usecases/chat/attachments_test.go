package chat_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

func pngBytes() []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
}

func TestUploadAttachment_StoresAndReturnsTheDurableRef(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	stored, err := f.usecase.UploadAttachment(f.ctx, chatID, agentusecase.UploadAttachmentInput{
		ID: "ab12", OriginalName: "photo.png", Data: pngBytes(),
	})
	require.NoError(t, err)
	assert.Equal(t, "chats/"+chatID+"/attachments/ab12-photo.png", stored.Ref)
	assert.Equal(t, "ab12-photo.png", stored.FileName)
	assert.Equal(t, "image/png", stored.ContentType)
	assert.Equal(t, len(pngBytes()), stored.Size)
}

func TestUploadAttachment_RefusesAChatThatDoesNotExist(t *testing.T) {
	f := newFixture(t)
	_, err := f.usecase.UploadAttachment(f.ctx, "no-such-chat", agentusecase.UploadAttachmentInput{
		ID: "ab12", OriginalName: "photo.png", Data: []byte("x"),
	})
	require.Error(t, err)
}

func TestUploadAttachment_ChatLookupFailure_ReturnsWrappedError(t *testing.T) {
	f, cs, _ := newFaultFixture(t)
	chatID, _ := f.spawn(t, "claude")

	cs.failGetChat = fmt.Errorf("boom: get chat")
	_, err := f.usecase.UploadAttachment(f.ctx, chatID, agentusecase.UploadAttachmentInput{
		ID: "ab12", OriginalName: "photo.png", Data: pngBytes(),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload attachment: chat")
}

func TestUploadAttachment_ChatsDirLookupFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	f.ws.err = fmt.Errorf("boom: worktree lookup")
	_, err := f.usecase.UploadAttachment(f.ctx, chatID, agentusecase.UploadAttachmentInput{
		ID: "ab12", OriginalName: "photo.png", Data: pngBytes(),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload attachment: chats dir")
}

func TestUploadAttachment_StoreFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.UploadAttachment(f.ctx, chatID, agentusecase.UploadAttachmentInput{
		ID: "not an id!", OriginalName: "photo.png", Data: pngBytes(),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, apperr.ErrInvalidArgument)
	assert.Contains(t, err.Error(), "upload attachment:")
}

func TestReadAttachment_RoundTripsAnUploadedFile(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	stored, err := f.usecase.UploadAttachment(f.ctx, chatID, agentusecase.UploadAttachmentInput{
		ID: "ab12", OriginalName: "photo.png", Data: pngBytes(),
	})
	require.NoError(t, err)

	data, contentType, err := f.usecase.ReadAttachment(f.ctx, chatID, stored.FileName)
	require.NoError(t, err)
	assert.Equal(t, pngBytes(), data)
	assert.Equal(t, "image/png", contentType)
}

func TestReadAttachment_MissingFileIsNotFound(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	_, _, err := f.usecase.ReadAttachment(f.ctx, chatID, "no-such-file.png")
	assert.ErrorIs(t, err, repoattachments.ErrNotFound)
}

func TestReadAttachment_ChatLookupFailure_ReturnsWrappedError(t *testing.T) {
	f, cs, _ := newFaultFixture(t)
	chatID, _ := f.spawn(t, "claude")

	cs.failGetChat = fmt.Errorf("boom: get chat")
	_, _, err := f.usecase.ReadAttachment(f.ctx, chatID, "no-such-file.png")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read attachment: chat")
}

func TestReadAttachment_ChatsDirLookupFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	f.ws.err = fmt.Errorf("boom: worktree lookup")
	_, _, err := f.usecase.ReadAttachment(f.ctx, chatID, "no-such-file.png")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read attachment: chats dir")
}
