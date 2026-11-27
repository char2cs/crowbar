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
