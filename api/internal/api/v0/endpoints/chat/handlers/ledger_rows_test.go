package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type fakeRows struct{ asked [2]int64 }

func (f *fakeRows) ReadRows(_ context.Context, chatID string, before, bytes int64) (agentusecase.RowsPage, error) {
	f.asked = [2]int64{before, bytes}
	if before < 1 {
		return agentusecase.RowsPage{}, fmt.Errorf("rows: %w", problem.ErrInvalidArgument)
	}
	if chatID != "chat-1" {
		return agentusecase.RowsPage{}, fmt.Errorf("rows: %w", problem.ErrNotFound)
	}
	return agentusecase.RowsPage{Rows: []ledger.Row{{ChatID: chatID, Seq: 8}}, From: 4}, nil
}

func (*fakeRows) ReadRow(_ context.Context, chatID string, seq int64) (ledger.Row, error) {
	if chatID != "chat-1" || seq != 8 {
		return ledger.Row{}, fmt.Errorf("row: %w", problem.ErrNotFound)
	}
	return ledger.Row{ChatID: chatID, Seq: seq}, nil
}

func getRows(t *testing.T, fake *fakeRows, path string) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handlers.NewLedgerRows(fake)
	r.GET("/chats/:chatId/rows", h.Page)
	r.GET("/chats/:chatId/rows/:seq", h.One)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, http.NoBody)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	return resp.StatusCode, body
}

func TestLedgerRows_PageAnswersTheTurnsBeforeARowAndWhetherMoreRemain(t *testing.T) {
	fake := &fakeRows{}

	status, body := getRows(t, fake, "/chats/chat-1/rows?before=9&bytes=1000")

	require.Equal(t, http.StatusOK, status)
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.InDelta(t, 4, data["from"], 0)
	assert.Equal(t, true, data["more"])
	assert.Len(t, data["rows"], 1)
	assert.Equal(t, [2]int64{9, 1000}, fake.asked)
}

func TestLedgerRows_PageDefaultsItsBytesAndRefusesAMissingBefore(t *testing.T) {
	fake := &fakeRows{}

	status, _ := getRows(t, fake, "/chats/chat-1/rows?before=9")
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, [2]int64{9, agentusecase.DefaultPageBytes}, fake.asked)

	status, _ = getRows(t, fake, "/chats/chat-1/rows")
	assert.Equal(t, http.StatusBadRequest, status)
	status, _ = getRows(t, fake, "/chats/chat-1/rows?before=x")
	assert.Equal(t, http.StatusBadRequest, status)
	status, _ = getRows(t, fake, "/chats/nope/rows?before=9")
	assert.Equal(t, http.StatusNotFound, status)
}

func TestLedgerRows_OneAnswersAWholeRowOrNotFound(t *testing.T) {
	status, body := getRows(t, &fakeRows{}, "/chats/chat-1/rows/8")
	require.Equal(t, http.StatusOK, status)
	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.InDelta(t, 8, data["seq"], 0)

	status, _ = getRows(t, &fakeRows{}, "/chats/chat-1/rows/9")
	assert.Equal(t, http.StatusNotFound, status)
	status, _ = getRows(t, &fakeRows{}, "/chats/chat-1/rows/x")
	assert.Equal(t, http.StatusBadRequest, status)
}
