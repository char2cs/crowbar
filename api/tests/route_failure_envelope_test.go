//go:build integration

package tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// Every route answers a failure from one mapping: the status of its problem
// code, the code itself, and the failure's own words without the wraps above it.
func TestRoute_AFailureCarriesItsCodeAndNotItsChain(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	chat := "/v0/chats/" + imported.chatID
	var thread struct {
		ID        string `json:"id"`
		MessageID string `json:"messageId"`
	}
	h.post(threadsBase(imported)+"/threads", map[string]any{
		"filePath": "README.md", "line": 1, "side": "right", "body": "root",
	}, http.StatusCreated, &thread)
	cases := []struct {
		name, method, path string
		body               any
		status             int
		code               problem.Code
		message            string
	}{
		{
			"unknown terminal", http.MethodDelete, chat + "/terminals/ghost", nil,
			http.StatusNotFound, problem.NotFound, "terminal: kill: terminal: session not found: ghost",
		},
		{
			"unknown thread", http.MethodGet, threadsBase(imported) + "/threads/ghost", nil,
			http.StatusNotFound, problem.NotFound, "reviewthread: get: asynx: aggregate not found",
		},
		{
			"the root through the message route", http.MethodDelete,
			threadsBase(imported) + "/threads/" + thread.ID + "/messages/" + thread.MessageID, nil,
			http.StatusUnprocessableEntity, problem.Unprocessable, "delete review message: cannot delete root comment: asynx: validation failed",
		},
		{
			"bad search pattern", http.MethodPost, chat + "/search",
			map[string]any{"query": "(", "regex": true},
			http.StatusBadRequest, problem.InvalidArgument, "invalid search pattern: error parsing regexp: missing closing ): `(?i)(`",
		},
		{
			"unknown review chat", http.MethodGet, "/v0/chats/ghost/review", nil,
			http.StatusNotFound, problem.NotFound, "chat not found",
		},
	}
	for _, tc := range cases {
		resp := h.raw(tc.method, tc.path, tc.body, tc.status)
		var env libs.Envelope
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&env), tc.name)
		require.NoError(t, resp.Body.Close())
		assert.Equal(t, tc.code, env.Code, tc.name)
		assert.Equal(t, tc.message, env.Error, tc.name)
	}
}
