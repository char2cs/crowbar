//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func operationSettled(op domain.Operation) bool {
	return op.State == domain.OpSucceeded || op.State == domain.OpFailed
}

// startOperation POSTs body (nil for none) under a fresh Idempotency-Key and
// returns the operation the 202 answers with.
func startOperation(
	t *testing.T,
	h *harness,
	path string,
	body any,
) domain.Operation {
	t.Helper()
	key := uuid.NewString()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, h.url+path, reader)
	require.NoError(t, err)
	req.Header.Set("Idempotency-Key", key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusAccepted, resp.StatusCode, path)
	var env struct {
		Data struct {
			Operation domain.Operation `json:"operation"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	require.Equal(t, key, env.Data.Operation.ID, "the operation id is the client's key")
	return env.Data.Operation
}

func TestOperations_SnapshotCarriesSettledOperationsAcrossARestart(t *testing.T) {
	h := newHarness(t)
	push := failingPush(t, h)
	op := startOperation(t, h, "/v0/chats/"+push.chatID+"/git/push", nil)
	failed := awaitOperation(t, h, op.ID, operationSettled)
	require.Equal(t, domain.OpFailed, failed.State)

	h.crash()
	again := newHarnessAt(t, h.home)

	restored := awaitOperation(t, again, op.ID, func(domain.Operation) bool { return true })
	require.Equal(t, domain.OpFailed, restored.State)
	require.Equal(t, failed.Error, restored.Error)
	require.Equal(t, failed.Version, restored.Version)
}

// A crash mid repo-delete leaves its row and its item running. The restart fails
// both as interrupted, and the delete it resumes settles its own item with it.
func TestOperations_ARestartSettlesTheItemsOfTheDeleteItCutShort(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)
	ctx := t.Context()
	repo, err := h.app.GORM.Repositories.FindByKey(ctx, f.repoID)
	require.NoError(t, err)
	require.NotNil(t, repo)
	repo.Deleting = true
	require.NoError(t, h.app.GORM.Repositories.Save(ctx, *repo))
	ref := domain.EntityRef{Kind: domain.EntityRepo, ID: f.repoID}
	cut := domain.Operation{
		ID: uuid.NewString(), Kind: domain.OpRepoDelete, Subject: ref, ProjectID: f.projectID,
		Label: "Remove " + repo.Name, State: domain.OpRunning, Version: 2,
		Items: []domain.OperationItem{{Ref: ref, Label: repo.Name, State: domain.OpRunning}},
	}
	require.NoError(t, h.app.GORM.Operations.Save(ctx, cut))

	h.crash()
	again := newHarnessAt(t, h.home)

	ops, err := again.app.Operations.Snapshot(ctx)
	require.NoError(t, err)
	var interrupted, resumed *domain.Operation
	for i := range ops {
		switch {
		case ops[i].ID == cut.ID:
			interrupted = &ops[i]
		case ops[i].Kind == domain.OpRepoDelete && ops[i].Subject == ref:
			resumed = &ops[i]
		}
	}
	require.NotNil(t, interrupted)
	require.Equal(t, domain.OpFailed, interrupted.State)
	require.Len(t, interrupted.Items, 1)
	require.Equal(t, domain.OpFailed, interrupted.Items[0].State, "no item runs on after its operation settled")
	require.NotNil(t, interrupted.Items[0].Error)
	require.Equal(t, problem.Interrupted, interrupted.Items[0].Error.Code)
	require.NotNil(t, resumed, "the boot resumes the delete")
	require.Equal(t, domain.OpSucceeded, resumed.State)
	require.Len(t, resumed.Items, 1)
	require.Equal(t, domain.OpSucceeded, resumed.Items[0].State)
}
