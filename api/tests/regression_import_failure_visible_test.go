//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// importBranches is the import-batch body for branches, each under a fresh chat id.
func importBranches(branches ...string) map[string]any {
	rows := make([]map[string]string, 0, len(branches))
	for _, b := range branches {
		rows = append(rows, map[string]string{"name": b, "chatId": uuid.NewString()})
	}
	return map[string]any{"branches": rows, "lock": false}
}

// postKeyed POSTs body under Idempotency-Key key and returns the status and body.
func postKeyed(
	t *testing.T,
	h *harness,
	path string,
	key string,
	body any,
) (int, []byte) {
	t.Helper()
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, h.url+path, bytes.NewReader(encoded))
	require.NoError(t, err)
	req.Header.Set("Idempotency-Key", key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, raw
}

func acceptedOperation(t *testing.T, status int, raw []byte) domain.Operation {
	t.Helper()
	require.Equal(t, http.StatusAccepted, status, string(raw))
	var env struct {
		Data struct {
			Operation domain.Operation `json:"operation"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &env))
	return env.Data.Operation
}

// badImport is an import of "bad/x", which git cannot check out, beside "good".
type badImport struct {
	fixture importFixture
	opID    string
	badChat string
	good    string
	body    map[string]any
}

// importWithOneBadBranch blocks bad/x's workspace root with a regular file
// named "bad", so `git worktree add` fails with "Not a directory".
func importWithOneBadBranch(t *testing.T, h *harness) badImport {
	t.Helper()
	f := newImportFixture(t, h)
	f.pushBranchToOrigin(t, "bad/x", "main", "bad\n")
	f.pushBranchToOrigin(t, "good", "main", "good\n")
	main, err := h.app.Repositories.Workspace.Get(t.Context(), f.mainWSID)
	require.NoError(t, err)
	slugDir := filepath.Dir(worktreepath.WorkspaceRoot(main.WorktreePath))
	require.NoError(t, os.WriteFile(filepath.Join(slugDir, "bad"), []byte("in the way\n"), 0o600))
	badChat, good := uuid.NewString(), uuid.NewString()
	return badImport{
		fixture: f,
		opID:    uuid.NewString(),
		badChat: badChat,
		good:    good,
		body: map[string]any{"branches": []map[string]string{
			{"name": "bad/x", "chatId": badChat},
			{"name": "good", "chatId": good},
		}},
	}
}

func (b badImport) send(t *testing.T, h *harness) domain.Operation {
	t.Helper()
	status, raw := b.fixture.post(t, h, b.opID, b.body)
	return acceptedOperation(t, status, raw)
}

func (f importFixture) post(t *testing.T, h *harness, key string, body any) (int, []byte) {
	t.Helper()
	return postKeyed(t, h, f.repoBase()+"/chats/import-batch", key, body)
}

func itemLabelled(t *testing.T, op domain.Operation, label string) domain.OperationItem {
	t.Helper()
	for _, item := range op.Items {
		if item.Label == label {
			return item
		}
	}
	t.Fatalf("operation %s has no item %q: %+v", op.ID, label, op.Items)
	return domain.OperationItem{}
}

func workspacesOnBranch(t *testing.T, h *harness, repoID, branch string) []domain.Workspace {
	t.Helper()
	h.Quiesce()
	all, err := h.app.Repositories.Workspace.List(t.Context())
	require.NoError(t, err)
	var out []domain.Workspace
	for _, ws := range all {
		if ws.RepoID == repoID && ws.Branch == branch && ws.Status != domain.WorkspaceStatusDeleted {
			out = append(out, ws)
		}
	}
	return out
}

func requireBadItemSaysWhy(t *testing.T, op domain.Operation) {
	t.Helper()
	bad := itemLabelled(t, op, "bad/x")
	require.Equal(t, domain.OpFailed, bad.State)
	require.NotNil(t, bad.Error)
	require.Contains(t, strings.ToLower(bad.Error.Message), "not a directory")
}

// TestRegression_BranchImportFailureIsVisible: an import whose branch cannot be checked out
// says which branch failed and why; the other branch lands.
func TestRegression_BranchImportFailureIsVisible(t *testing.T) {
	h := newHarness(t)
	b := importWithOneBadBranch(t, h)

	op := b.send(t, h)
	require.Equal(t, b.opID, op.ID)
	require.Equal(t, domain.OpBranchImport, op.Kind)
	require.Len(t, op.Items, 2)
	for _, item := range op.Items {
		require.Equal(t, domain.OpRunning, item.State)
	}

	settled := awaitOperation(t, h, op.ID, operationSettled)
	require.Equal(t, domain.OpFailed, settled.State)
	require.NotNil(t, settled.Error)
	require.Equal(t, problem.Partial, settled.Error.Code)
	requireBadItemSaysWhy(t, settled)
	good := itemLabelled(t, settled, "good")
	require.Equal(t, domain.OpSucceeded, good.State)
	require.Equal(t, b.good, good.Ref.ID)

	goodWS := workspacesOnBranch(t, h, b.fixture.repoID, "good")
	require.Len(t, goodWS, 1)
	require.NotEmpty(t, goodWS[0].WorktreePath)
	require.Equal(t, b.good, owningChatID(t, h, goodWS[0].ID))
	badWS := workspacesOnBranch(t, h, b.fixture.repoID, "bad/x")
	require.Len(t, badWS, 1)
	require.Empty(t, badWS[0].WorktreePath, "the failed branch is a placeholder")
	require.Equal(t, b.badChat, owningChatID(t, h, badWS[0].ID), "owned by the chat the client drew it as")
}

// TestRegression_BranchImportReasonSurvivesRestart: the reason is still there after a restart.
func TestRegression_BranchImportReasonSurvivesRestart(t *testing.T) {
	h := newHarness(t)
	b := importWithOneBadBranch(t, h)
	op := b.send(t, h)
	before := awaitOperation(t, h, op.ID, operationSettled)

	h.crash()
	again := newHarnessAt(t, h.home)

	restored := awaitOperation(t, again, op.ID, func(domain.Operation) bool { return true })
	requireBadItemSaysWhy(t, restored)
	require.Equal(t, itemLabelled(t, before, "bad/x").Error, itemLabelled(t, restored, "bad/x").Error)
}

// TestRegression_BranchImportRetryReturnsTheSameOperation: the same import sent twice
// (a double click, a retried request) is one operation and one workspace per branch.
func TestRegression_BranchImportRetryReturnsTheSameOperation(t *testing.T) {
	h := newHarness(t)
	b := importWithOneBadBranch(t, h)

	first := b.send(t, h)
	awaitOperation(t, h, first.ID, operationSettled)
	again := b.send(t, h)

	require.Equal(t, first.ID, again.ID)
	require.Len(t, workspacesOnBranch(t, h, b.fixture.repoID, "good"), 1)
}

// TestRegression_ImportRefusesATakenChatID: a chat id already in use is refused before
// any work starts.
func TestRegression_ImportRefusesATakenChatID(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)
	f.pushBranchToOrigin(t, "good", "main", "good\n")
	taken := owningChatID(t, h, f.mainWSID)
	key := uuid.NewString()

	status, raw := f.post(t, h, key, map[string]any{"branches": []map[string]string{{"name": "good", "chatId": taken}}})

	require.Equal(t, http.StatusConflict, status, string(raw))
	require.Contains(t, string(raw), string(problem.IDConflict))
	ops, err := h.app.Operations.Snapshot(t.Context())
	require.NoError(t, err)
	for _, op := range ops {
		require.NotEqual(t, key, op.ID, "a refused import records no operation")
	}
}
