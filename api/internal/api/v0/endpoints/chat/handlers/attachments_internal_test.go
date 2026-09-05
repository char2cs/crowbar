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

// brokenReader always fails — the fake/broken io.Reader idiom this package's
// own ReadAttachment tests (and the attachments repository's chmod-based
// failure injections) already use for a genuinely-reachable I/O error that a
// real file wouldn't reliably reproduce.
type brokenReader struct{ err error }

func (r brokenReader) Read([]byte) (int, error) { return 0, r.err }

func newAttachmentTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	return ctx
}

// TestReadAttachmentBytes_ReadErrorIsInternalServerError pins
// readAttachmentBytes' io.ReadAll failure branch — the ONE read-and-check
// both readAttachmentFromMultipart's and readAttachmentFromPath's io.ReadAll
// calls now share, so this one test covers both named branches at once.
func TestReadAttachmentBytes_ReadErrorIsInternalServerError(t *testing.T) {
	ctx := newAttachmentTestContext()

	data, ok := readAttachmentBytes(ctx, brokenReader{err: errors.New("disk yanked")})

	assert.False(t, ok)
	assert.Nil(t, data)
}

// TestReadAttachmentBytes_OverCapStreamIsEntityTooLarge pins the post-read
// size-check branch: a stream that actually yields more than MaxBytes — the
// path variant's real-world case is a file that grows between its own
// os.Stat and the read that follows, so the earlier size check no longer
// bounds what io.ReadAll actually returns.
func TestReadAttachmentBytes_OverCapStreamIsEntityTooLarge(t *testing.T) {
	ctx := newAttachmentTestContext()
	oversized := bytes.NewReader(bytes.Repeat([]byte("a"), int(repoattachments.MaxBytes)+1))

	data, ok := readAttachmentBytes(ctx, oversized)

	assert.False(t, ok)
	assert.Nil(t, data)
}

// TestReadAttachmentBytes_ExactlyAtCapSucceeds is the non-vacuousness
// checkpoint for the test above: MaxBytes itself must still pass, so the
// failure above is genuinely about exceeding the cap, not an off-by-one that
// would reject everything.
func TestReadAttachmentBytes_ExactlyAtCapSucceeds(t *testing.T) {
	ctx := newAttachmentTestContext()
	atCap := bytes.NewReader(bytes.Repeat([]byte("a"), int(repoattachments.MaxBytes)))

	data, ok := readAttachmentBytes(ctx, atCap)

	require.True(t, ok)
	assert.Len(t, data, int(repoattachments.MaxBytes))
}

// TestResolveOriginalName_KeepsAGivenFilename proves a client-supplied
// filename passes through verbatim.
func TestResolveOriginalName_KeepsAGivenFilename(t *testing.T) {
	assert.Equal(t, "photo.png", resolveOriginalName("photo.png", []byte("data")))
}

// TestResolveOriginalName_SynthesizesFromContentTypeWhenBlank proves the
// backstop branch net/http's own multipart parser can never itself trigger
// (a FILE part with an explicitly empty filename is reclassified as a form
// VALUE before this code ever runs — see attachments_test.go's own note) —
// exercised directly here so the defensive branch still has real coverage.
func TestResolveOriginalName_SynthesizesFromContentTypeWhenBlank(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)
	assert.Regexp(t, `^pasted-image-.+\.png$`, resolveOriginalName("", png))
}
