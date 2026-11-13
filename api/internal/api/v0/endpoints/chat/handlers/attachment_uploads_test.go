package handlers_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// chunkedUploadFake records each chunked-upload call as "op id[ detail]" and answers canned values.
type chunkedUploadFake struct {
	calls    []string
	received int64
	err      error
}

func (f *fakeAgentUsecase) BeginAttachmentUpload(_ context.Context, _, id, fileName string, size int64) (int64, error) {
	f.chunks.calls = append(f.chunks.calls, fmt.Sprintf("begin %s %s %d", id, fileName, size))
	return f.chunks.received, f.chunks.err
}

func (f *fakeAgentUsecase) AppendAttachmentUpload(_ context.Context, _, id string, offset int64, r io.Reader) (int64, error) {
	chunk, _ := io.ReadAll(r)
	f.chunks.calls = append(f.chunks.calls, fmt.Sprintf("append %s %d %s", id, offset, chunk))
	return offset + int64(len(chunk)), f.chunks.err
}

func (f *fakeAgentUsecase) CompleteAttachmentUpload(_ context.Context, _, id, fileName string, size int64) (agentusecase.StoredAttachment, error) {
	f.chunks.calls = append(f.chunks.calls, fmt.Sprintf("complete %s %s %d", id, fileName, size))
	return f.uploadAttachmentOut, f.chunks.err
}

func (f *fakeAgentUsecase) AbortAttachmentUpload(_ context.Context, _, id string) error {
	f.chunks.calls = append(f.chunks.calls, "abort "+id)
	return f.chunks.err
}

func uploadCtx(t *testing.T, method, target, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	ctx, rec := newTestContext(t, method, target, []byte(body))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "chat-1"}, {Key: "uploadId", Value: "ab12"}}
	return ctx, rec
}

func TestBeginAttachmentUpload_AnswersTheBytesAlreadyHeld(t *testing.T) {
	uc := inWorkspace(&fakeAgentUsecase{chunks: chunkedUploadFake{received: 4}})
	ctx, rec := uploadCtx(t, http.MethodPost, "/uploads", `{"id":"ab12","fileName":"a.bin","size":9}`)

	newChatHandlers(uc).BeginAttachmentUpload(ctx)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"success":true,"data":{"received":4}}`, rec.Body.String())
	assert.Equal(t, []string{"begin ab12 a.bin 9"}, uc.chunks.calls)
}

func TestBeginAttachmentUpload_RefusesAMalformedBody(t *testing.T) {
	uc := inWorkspace(&fakeAgentUsecase{})
	ctx, rec := uploadCtx(t, http.MethodPost, "/uploads", `{"fileName":"a.bin"}`)

	newChatHandlers(uc).BeginAttachmentUpload(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, uc.chunks.calls)
}

func TestAppendAttachmentUpload_PassesTheOffsetAndBodyThrough(t *testing.T) {
	uc := inWorkspace(&fakeAgentUsecase{})
	ctx, rec := uploadCtx(t, http.MethodPut, "/uploads/ab12?offset=3", "xyz")

	newChatHandlers(uc).AppendAttachmentUpload(ctx)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"success":true,"data":{"received":6}}`, rec.Body.String())
	assert.Equal(t, []string{"append ab12 3 xyz"}, uc.chunks.calls)
}

func TestAppendAttachmentUpload_RefusesABadOffset(t *testing.T) {
	uc := inWorkspace(&fakeAgentUsecase{})
	ctx, rec := uploadCtx(t, http.MethodPut, "/uploads/ab12?offset=-1", "xyz")

	newChatHandlers(uc).AppendAttachmentUpload(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, uc.chunks.calls)
}

func TestAppendAttachmentUpload_SurfacesAConflict(t *testing.T) {
	uc := inWorkspace(&fakeAgentUsecase{chunks: chunkedUploadFake{err: problem.ErrConflict}})
	ctx, rec := uploadCtx(t, http.MethodPut, "/uploads/ab12?offset=0", "xyz")

	newChatHandlers(uc).AppendAttachmentUpload(ctx)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestCompleteAttachmentUpload_AnswersTheSameRecordAsASingleShotUpload(t *testing.T) {
	uc := inWorkspace(&fakeAgentUsecase{uploadAttachmentOut: agentusecase.StoredAttachment{
		Ref: "chats/chat-1/attachments/ab12-a.bin", FileName: "ab12-a.bin", Size: 9, ContentType: "application/octet-stream",
	}})
	ctx, rec := uploadCtx(t, http.MethodPost, "/uploads/ab12/complete", `{"fileName":"a.bin","size":9}`)

	newChatHandlers(uc).CompleteAttachmentUpload(ctx)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"fileName":"ab12-a.bin"`)
	assert.Equal(t, []string{"complete ab12 a.bin 9"}, uc.chunks.calls)
}

func TestAbortAttachmentUpload_AnswersNoContent(t *testing.T) {
	uc := inWorkspace(&fakeAgentUsecase{})
	ctx, rec := uploadCtx(t, http.MethodDelete, "/uploads/ab12", "")

	newChatHandlers(uc).AbortAttachmentUpload(ctx)
	ctx.Writer.WriteHeaderNow()

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, []string{"abort ab12"}, uc.chunks.calls)
}

func (configurableListGetUsecase) BeginAttachmentUpload(context.Context, string, string, string, int64) (int64, error) {
	return 0, nil
}

func (configurableListGetUsecase) AppendAttachmentUpload(context.Context, string, string, int64, io.Reader) (int64, error) {
	return 0, nil
}

func (configurableListGetUsecase) CompleteAttachmentUpload(
	context.Context, string, string, string, int64,
) (agentusecase.StoredAttachment, error) {
	return agentusecase.StoredAttachment{}, nil
}

func (configurableListGetUsecase) AbortAttachmentUpload(context.Context, string, string) error {
	return nil
}
