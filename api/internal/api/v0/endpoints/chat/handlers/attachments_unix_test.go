//go:build unix

package handlers_test

import (
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
)

// TestUploadAttachment_JSONPathVariantGrowsPastCapBetweenStatAndRead pins
// readAttachmentFromPath's post-read size-check branch — the same
// "Hardening applied" guarantee icons.ReadUpload's own
// TestReadUploadRefusesAPathThatGrowsPastTheCapBetweenStatAndRead pins for
// its sibling endpoint. A FIFO makes this deterministic without a real
// race: os.Stat reports a pipe's size as 0 (always under the cap), so the
// pre-read size check passes; the actual bytes read past the writer's other
// end are what trip the post-read oversize guard. Opening one end of a FIFO
// blocks until the other end opens, so the writer goroutine's ordering
// relative to the handler never matters — there is no interval to guess at.
func TestUploadAttachment_JSONPathVariantGrowsPastCapBetweenStatAndRead(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "growing")
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		w, err := os.OpenFile(fifo, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		defer func() { _ = w.Close() }()
		// One byte past MaxBytes+1 so the LimitReader-capped read still comes
		// back oversize regardless of how many bytes actually land.
		_, _ = w.Write(make([]byte, repoattachments.MaxBytes+2))
	}()
	t.Cleanup(wg.Wait)

	reqBody := []byte(`{"path":"` + fifo + `","id":"ab12"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/attachments", reqBody)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}

	uc := inWorkspace(&fakeAgentUsecase{})
	newChatHandlers(uc).UploadAttachment(ctx)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
	assert.Empty(t, uc.uploadAttachmentCalls)
}
