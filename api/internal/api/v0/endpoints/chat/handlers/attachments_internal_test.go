package handlers

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
)

type brokenReader struct{ err error }

func (r brokenReader) Read([]byte) (int, error) { return 0, r.err }

func newAttachmentTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	return ctx
}

func TestReadAttachmentBytes_ReadErrorIsInternalServerError(t *testing.T) {
	ctx := newAttachmentTestContext()

	data, ok := readAttachmentBytes(ctx, brokenReader{err: errors.New("disk yanked")})

	assert.False(t, ok)
	assert.Nil(t, data)
}

func TestReadAttachmentBytes_OverCapStreamIsEntityTooLarge(t *testing.T) {
	ctx := newAttachmentTestContext()
	oversized := bytes.NewReader(bytes.Repeat([]byte("a"), int(repoattachments.MaxBytes)+1))

	data, ok := readAttachmentBytes(ctx, oversized)

	assert.False(t, ok)
	assert.Nil(t, data)
}

func TestReadAttachmentBytes_ExactlyAtCapSucceeds(t *testing.T) {
	ctx := newAttachmentTestContext()
	atCap := bytes.NewReader(bytes.Repeat([]byte("a"), int(repoattachments.MaxBytes)))

	data, ok := readAttachmentBytes(ctx, atCap)

	require.True(t, ok)
	assert.Len(t, data, int(repoattachments.MaxBytes))
}

func TestResolveOriginalName_KeepsAGivenFilename(t *testing.T) {
	assert.Equal(t, "photo.png", resolveOriginalName("photo.png", []byte("data")))
}

func TestResolveOriginalName_SynthesizesFromContentTypeWhenBlank(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
	assert.Regexp(t, `^pasted-image-.+\.png$`, resolveOriginalName("", png))
}
