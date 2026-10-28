package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// accepted decodes a 202's operation.
func accepted(t *testing.T, rec *httptest.ResponseRecorder) domain.Operation {
	t.Helper()
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	var env struct {
		Data struct {
			Operation domain.Operation `json:"operation"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	return env.Data.Operation
}

func TestRemoteVerbs_AnswerTheirOperation(t *testing.T) {
	r := newRouter()

	for verb, kind := range map[string]domain.OperationKind{
		"push": domain.OpGitPush, "fetch": domain.OpGitFetch, "pull": domain.OpGitPull,
	} {
		op := accepted(t, do(r, http.MethodPost, ws+"/"+verb, nil))
		assert.Equal(t, kind, op.Kind, verb)
		assert.Equal(t, domain.EntityRef{Kind: domain.EntityWorkspace, ID: "ws1"}, op.Subject, verb)
		assert.Equal(t, domain.OpAccepted, op.State, verb)
	}
}

func TestPush_TheOperationIdIsTheIdempotencyKey(t *testing.T) {
	r := newRouter()
	key := uuid.NewString()
	send := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, ws+"/push", bytes.NewReader(nil))
		req.Header.Set("Idempotency-Key", key)
		r.ServeHTTP(rec, req)
		return rec
	}

	first := accepted(t, send())
	again := accepted(t, send())

	assert.Equal(t, key, first.ID)
	assert.Equal(t, key, again.ID)
}

func TestPush_AMalformedKeyIs400(t *testing.T) {
	r := newRouter()
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, ws+"/push", nil)
	req.Header.Set("Idempotency-Key", "x")
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// A push that fails after its 202 settles its operation with the reason.
func TestPush_AFailureSettlesTheOperationWithItsReason(t *testing.T) {
	frames := make(chan domain.Operation, 16)
	r := newRouterWithOps(errGit{}, newOps(func(op domain.Operation) { frames <- op }))

	op := accepted(t, do(r, http.MethodPost, ws+"/push", nil))

	for frame := range frames {
		if frame.ID != op.ID || frame.State != domain.OpFailed {
			continue
		}
		require.NotNil(t, frame.Error)
		assert.Equal(t, "boom", frame.Error.Message)
		return
	}
}
