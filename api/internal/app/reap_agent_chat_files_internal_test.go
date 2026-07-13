package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agent"
)

// fakeAgentWorkspaceReader is a minimal agent.WorkspaceReader double:
// AgentChatsDir/WorktreeDir return preset values (or a preset error), letting
// reapAgentChatFiles be exercised without a real workspace read model — the
// same role fakeWorkspacePaths plays for bootSweepPurge in
// boot_sweep_internal_test.go.
type fakeAgentWorkspaceReader struct {
	chatsDir    string
	chatsDirErr error
	home        string
	homeErr     error
}

func (f *fakeAgentWorkspaceReader) WorktreeDir(
	_ context.Context,
	_ string,
) (crowbarHome, projectID, repoID, worktree string, err error) {
	if f.homeErr != nil {
		return "", "", "", "", f.homeErr
	}
	return f.home, "p1", "r1", "", nil
}

func (f *fakeAgentWorkspaceReader) AgentChatsDir(
	_ context.Context,
	_ string,
) (string, error) {
	if f.chatsDirErr != nil {
		return "", f.chatsDirErr
	}
	return f.chatsDir, nil
}

var _ agent.WorkspaceReader = (*fakeAgentWorkspaceReader)(nil)

// TestReapAgentChatFiles_RemovesChatDir_LeavesSiblingsIntact pins the happy
// path the workspace-delete cascade relies on: reapAgentChatFiles removes ONLY
// the target chat's own <chatsDir>/<chatID> directory, leaving a sibling
// chat's directory (as if it belonged to another workspace sharing the SAME
// chats dir, the home-kind sharing scenario the safety rule guards) and the
// shared chats dir parent itself untouched.
func TestReapAgentChatFiles_RemovesChatDir_LeavesSiblingsIntact(t *testing.T) {
	home := t.TempDir()
	chatsDir := filepath.Join(home, "projects", "p1", "github.com", "acme", "repo", "default", "chats")
	target := filepath.Join(chatsDir, "chat1")
	sibling := filepath.Join(chatsDir, "chat-other")
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.MkdirAll(sibling, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "ledger.jsonl"), []byte("{}"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(sibling, "ledger.jsonl"), []byte("{}"), 0o600))

	reap := reapAgentChatFiles(&fakeAgentWorkspaceReader{chatsDir: chatsDir, home: home})
	require.NoError(t, reap(context.Background(), "w1", "chat1"))

	_, err := os.Stat(target)
	assert.True(t, os.IsNotExist(err), "the reaped chat's own directory must be removed")
	_, err = os.Stat(sibling)
	assert.NoError(t, err, "a sibling chat directory must survive")
	_, err = os.Stat(chatsDir)
	assert.NoError(t, err, "the shared chats dir parent must survive")
}

// TestReapAgentChatFiles_RefusesPathOutsideCrowbarHome pins the safety
// guarantee reused from agent.RemoveUnderHome (Task 7): a poisoned or
// otherwise external AgentChatsDir resolution must never be removed, even
// though reapAgentChatFiles itself does no additional guarding beyond routing
// through RemoveUnderHome.
func TestReapAgentChatFiles_RefusesPathOutsideCrowbarHome(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir() // NOT under home: a poisoned/external chats dir.
	target := filepath.Join(outside, "chat1")
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "sentinel"), []byte("x"), 0o600))

	reap := reapAgentChatFiles(&fakeAgentWorkspaceReader{chatsDir: outside, home: home})
	require.NoError(t, reap(context.Background(), "w1", "chat1"))

	_, err := os.Stat(filepath.Join(target, "sentinel"))
	assert.NoError(t, err, "a chats dir outside the crowbar home must never be removed")
}

// TestReapAgentChatFiles_ChatsDirResolveError_ReturnsError pins that a failure
// to resolve the chats dir is surfaced as an error (so the repositories-layer
// cascade can log it) rather than silently swallowed here.
func TestReapAgentChatFiles_ChatsDirResolveError_ReturnsError(t *testing.T) {
	reap := reapAgentChatFiles(&fakeAgentWorkspaceReader{chatsDirErr: errors.New("boom")})
	err := reap(context.Background(), "w1", "chat1")
	assert.Error(t, err)
}

// TestReapAgentChatFiles_HomeResolveError_ReturnsError pins the same contract
// for a failure to resolve crowbar home.
func TestReapAgentChatFiles_HomeResolveError_ReturnsError(t *testing.T) {
	reap := reapAgentChatFiles(&fakeAgentWorkspaceReader{chatsDir: "/some/dir", homeErr: errors.New("boom")})
	err := reap(context.Background(), "w1", "chat1")
	assert.Error(t, err)
}
