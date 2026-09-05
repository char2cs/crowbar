package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type uploadAttachmentCall struct {
	chatID string
	in     agentusecase.UploadAttachmentInput
}

type readAttachmentCall struct{ chatID, fileName string }

func (f *fakeAgentUsecase) UploadAttachment(
	_ context.Context, chatID string, in agentusecase.UploadAttachmentInput,
) (agentusecase.StoredAttachment, error) {
	f.uploadAttachmentCalls = append(f.uploadAttachmentCalls, uploadAttachmentCall{chatID: chatID, in: in})
	return f.uploadAttachmentOut, f.uploadAttachmentErr
}

func (f *fakeAgentUsecase) ReadAttachment(
	_ context.Context, chatID, fileName string,
) ([]byte, string, error) {
	f.readAttachmentCalls = append(f.readAttachmentCalls, readAttachmentCall{chatID: chatID, fileName: fileName})
	return f.readAttachmentData, f.readAttachmentType, f.readAttachmentErr
}

func multipartAttachmentBody(t *testing.T, id, fileName string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	require.NoError(t, w.WriteField("id", id))
	fw, err := w.CreateFormFile("file", fileName)
	require.NoError(t, err)
	_, err = fw.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf, w.FormDataContentType()
}

func TestUploadAttachment_MultipartStoresAndReturnsTheRef(t *testing.T) {
	uc := inWorkspace(&fakeAgentUsecase{
		uploadAttachmentOut: agentusecase.StoredAttachment{
			Ref: "chats/chat-1/attachments/ab12-photo.png", FileName: "ab12-photo.png",
			Size: 4, ContentType: "image/png",
		},
	})
	body, contentType := multipartAttachmentBody(t, "ab12", "photo.png", []byte("data"))
	ctx, rec := newTestContext(t, http.MethodPost, "/attachments", nil)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/attachments", body)
	ctx.Request.Header.Set("Content-Type", contentType)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}

	newChatHandlers(uc).UploadAttachment(ctx)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var out struct {
		Data dto.ChatAttachmentDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "chats/chat-1/attachments/ab12-photo.png", out.Data.Ref)
	assert.Equal(t, "ab12-photo.png", out.Data.FileName)
	assert.Equal(t, 4, out.Data.Size)
	assert.Equal(t, "image/png", out.Data.ContentType)
	require.Len(t, uc.uploadAttachmentCalls, 1)
	assert.Equal(t, "chat-1", uc.uploadAttachmentCalls[0].chatID)
	assert.Equal(t, "ab12", uc.uploadAttachmentCalls[0].in.ID)
	assert.Equal(t, "photo.png", uc.uploadAttachmentCalls[0].in.OriginalName)
	assert.Equal(t, []byte("data"), uc.uploadAttachmentCalls[0].in.Data)
}

func TestUploadAttachment_MultipartRequiresAnID(t *testing.T) {
	body, contentType := multipartAttachmentBody(t, "", "photo.png", []byte("data"))
	ctx, rec := newTestContext(t, http.MethodPost, "/attachments", nil)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/attachments", body)
	ctx.Request.Header.Set("Content-Type", contentType)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}

	uc := inWorkspace(&fakeAgentUsecase{})
	newChatHandlers(uc).UploadAttachment(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, uc.uploadAttachmentCalls)
}

func TestUploadAttachment_MultipartRequiresAFileField(t *testing.T) {
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	require.NoError(t, w.WriteField("id", "ab12"))
	require.NoError(t, w.Close())
	ctx, rec := newTestContext(t, http.MethodPost, "/attachments", nil)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/attachments", buf)
	ctx.Request.Header.Set("Content-Type", w.FormDataContentType())
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}

	uc := inWorkspace(&fakeAgentUsecase{})
	newChatHandlers(uc).UploadAttachment(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, uc.uploadAttachmentCalls)
}

func TestUploadAttachment_MultipartOversizedBodyIs413(t *testing.T) {
	oversized := bytes.Repeat([]byte("a"), repoattachments.MaxBytes+1)
	body, contentType := multipartAttachmentBody(t, "ab12", "big.bin", oversized)
	ctx, rec := newTestContext(t, http.MethodPost, "/attachments", nil)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/attachments", body)
	ctx.Request.Header.Set("Content-Type", contentType)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}

	uc := inWorkspace(&fakeAgentUsecase{})
	newChatHandlers(uc).UploadAttachment(ctx)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Empty(t, uc.uploadAttachmentCalls)
}

func TestUploadAttachment_JSONPathVariantReadsTheHostFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/dropped.png"
	require.NoError(t, os.WriteFile(path, []byte("dropped bytes"), 0o600))

	uc := inWorkspace(&fakeAgentUsecase{
		uploadAttachmentOut: agentusecase.StoredAttachment{Ref: "chats/chat-1/attachments/ab12-dropped.png"},
	})
	reqBody := []byte(`{"path":"` + path + `","id":"ab12"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/attachments", reqBody)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}

	newChatHandlers(uc).UploadAttachment(ctx)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Len(t, uc.uploadAttachmentCalls, 1)
	assert.Equal(t, "ab12", uc.uploadAttachmentCalls[0].in.ID)
	assert.Equal(t, "dropped.png", uc.uploadAttachmentCalls[0].in.OriginalName)
	assert.Equal(t, []byte("dropped bytes"), uc.uploadAttachmentCalls[0].in.Data)
}

func TestUploadAttachment_JSONPathVariantRequiresPathAndID(t *testing.T) {
	for _, body := range [][]byte{
		[]byte(`{"path":"","id":"ab12"}`),
		[]byte(`{"path":"/tmp/x","id":""}`),
		[]byte(`{not json`),
	} {
		ctx, rec := newTestContext(t, http.MethodPost, "/attachments", body)
		ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}

		uc := inWorkspace(&fakeAgentUsecase{})
		newChatHandlers(uc).UploadAttachment(ctx)

		assert.Equal(t, http.StatusBadRequest, rec.Code, string(body))
		assert.Empty(t, uc.uploadAttachmentCalls, string(body))
	}
}

func TestUploadAttachment_JSONPathVariantMissingFileIs400(t *testing.T) {
	reqBody := []byte(`{"path":"/no/such/file-ever","id":"ab12"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/attachments", reqBody)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}

	uc := inWorkspace(&fakeAgentUsecase{})
	newChatHandlers(uc).UploadAttachment(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, uc.uploadAttachmentCalls)
}

func TestUploadAttachment_JSONPathVariantRefusesADirectory(t *testing.T) {
	dir := t.TempDir()

	reqBody := []byte(`{"path":"` + dir + `","id":"ab12"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/attachments", reqBody)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}

	uc := inWorkspace(&fakeAgentUsecase{})
	newChatHandlers(uc).UploadAttachment(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Empty(t, uc.uploadAttachmentCalls)
}

// TestUploadAttachment_JSONPathVariantUnreadableFileIs400 pins the
// os.Open failure branch: os.Stat succeeds (the file exists, so the earlier
// size check passes) but the file itself is unreadable — the same
// permission-based failure injection attachments_test.go's own
// TestRead_OpenErrorReturnsNotFound uses for the repository's os.Open call.
func TestUploadAttachment_JSONPathVariantUnreadableFileIs400(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/forbidden.png"
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o000))
	defer func() { _ = os.Chmod(path, 0o644) }() // restore for cleanup

	reqBody := []byte(`{"path":"` + path + `","id":"ab12"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/attachments", reqBody)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}

	uc := inWorkspace(&fakeAgentUsecase{})
	newChatHandlers(uc).UploadAttachment(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Empty(t, uc.uploadAttachmentCalls)
}

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
	if runtime.GOOS == "windows" {
		t.Skip("named pipes are unix-only")
	}
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

func TestUploadAttachment_JSONPathVariantOversizedFileIs413(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/big.bin"
	require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte("a"), repoattachments.MaxBytes+1), 0o600))

	reqBody := []byte(`{"path":"` + path + `","id":"ab12"}`)
	ctx, rec := newTestContext(t, http.MethodPost, "/attachments", reqBody)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}

	uc := inWorkspace(&fakeAgentUsecase{})
	newChatHandlers(uc).UploadAttachment(ctx)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Empty(t, uc.uploadAttachmentCalls)
}

func TestUploadAttachment_RefusesAChatOutsideTheRouteScope(t *testing.T) {
	uc := &fakeAgentUsecase{}
	body, contentType := multipartAttachmentBody(t, "ab12", "photo.png", []byte("data"))
	ctx, rec := newTestContext(t, http.MethodPost, "/attachments", nil)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/attachments", body)
	ctx.Request.Header.Set("Content-Type", contentType)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}

	newChatHandlers(uc).UploadAttachment(ctx)

	assert.NotEqual(t, http.StatusCreated, rec.Code)
	assert.Empty(t, uc.uploadAttachmentCalls)
}

func TestUploadAttachment_SurfacesAUsecaseFailure(t *testing.T) {
	uc := inWorkspace(&fakeAgentUsecase{uploadAttachmentErr: errors.New("store unavailable")})
	body, contentType := multipartAttachmentBody(t, "ab12", "photo.png", []byte("data"))
	ctx, rec := newTestContext(t, http.MethodPost, "/attachments", nil)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/attachments", body)
	ctx.Request.Header.Set("Content-Type", contentType)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}}

	newChatHandlers(uc).UploadAttachment(ctx)

	assert.GreaterOrEqual(t, rec.Code, http.StatusInternalServerError)
}

func TestAttachment_ServesRawBytesWithTheSniffedContentType(t *testing.T) {
	uc := inWorkspace(&fakeAgentUsecase{
		readAttachmentData: []byte("raw bytes"), readAttachmentType: "text/plain; charset=utf-8",
	})
	ctx, rec := scoped(t, "/attachments/ab12-notes.txt")
	ctx.Params = append(ctx.Params, gin.Param{Key: "file", Value: "ab12-notes.txt"})

	newChatHandlers(uc).Attachment(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "raw bytes", rec.Body.String())
	assert.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))
	require.Len(t, uc.readAttachmentCalls, 1)
	assert.Equal(t, "chat-1", uc.readAttachmentCalls[0].chatID)
	assert.Equal(t, "ab12-notes.txt", uc.readAttachmentCalls[0].fileName)
}

func TestAttachment_MissingFileIs404(t *testing.T) {
	uc := inWorkspace(&fakeAgentUsecase{readAttachmentErr: repoattachments.ErrNotFound})
	ctx, rec := scoped(t, "/attachments/no-such-file.png")
	ctx.Params = append(ctx.Params, gin.Param{Key: "file", Value: "no-such-file.png"})

	newChatHandlers(uc).Attachment(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestAttachment_RefusesAChatOutsideTheRouteScope(t *testing.T) {
	uc := &fakeAgentUsecase{getChat: domain.Chat{ID: "chat-1", WorkspaceID: "another-ws"}}
	ctx, rec := scoped(t, "/attachments/notes.txt")
	ctx.Params = append(ctx.Params, gin.Param{Key: "file", Value: "notes.txt"})

	newChatHandlers(uc).Attachment(ctx)

	assert.NotEqual(t, http.StatusOK, rec.Code)
	assert.Empty(t, uc.readAttachmentCalls)
}
