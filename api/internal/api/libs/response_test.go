package libs_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/libs"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

type envelope struct {
	Success bool    `json:"success"`
	Error   *string `json:"error"`
	ID      string  `json:"id"`
	Data    any     `json:"data"`
}

func newContext(method, path string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, nil)
	return c, w
}

func TestWriteMutationOK(t *testing.T) {
	c, w := newContext(http.MethodPost, "/")
	libs.WriteMutationOK(c, http.StatusCreated, "proj-123")

	assert.Equal(t, http.StatusCreated, w.Code)
	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.True(t, env.Success)
	assert.Nil(t, env.Error)
	assert.Equal(t, "proj-123", env.ID)
}

func TestWriteQueryOK(t *testing.T) {
	c, w := newContext(http.MethodGet, "/")
	libs.WriteQueryOK(c, map[string]string{"key": "val"})

	assert.Equal(t, http.StatusOK, w.Code)
	var env map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.True(t, env["success"].(bool))
	assert.NotNil(t, env["data"])
}

func TestWriteErr(t *testing.T) {
	c, w := newContext(http.MethodPost, "/")
	libs.WriteErr(c, http.StatusNotFound, "not found", "proj-123")

	assert.Equal(t, http.StatusNotFound, w.Code)
	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.False(t, env.Success)
	require.NotNil(t, env.Error)
	assert.Equal(t, "not found", *env.Error)
	assert.Equal(t, "proj-123", env.ID)
}

func TestWriteErr_NoID(t *testing.T) {
	c, w := newContext(http.MethodGet, "/")
	libs.WriteErr(c, http.StatusInternalServerError, "internal error", "")

	var env map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	_, hasID := env["id"]
	assert.False(t, hasID, "id should be omitted when empty")
}

func TestDataResponse(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	resp := libs.DataResponse(payload{Name: "test"})
	b, err := json.Marshal(resp)
	require.NoError(t, err)

	var env map[string]any
	require.NoError(t, json.Unmarshal(b, &env))
	assert.True(t, env["success"].(bool))
	assert.NotNil(t, env["data"])
}
