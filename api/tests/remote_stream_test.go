//go:build integration

package tests

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// A paired client over TLS holds the same scopes the local socket does.
func TestRemoteStream_ServesProjectsAndWorkspaceScopes(t *testing.T) {
	h := newPairedHarness(t)
	imported := importWritableWorkspace(t, h.harness)
	c, status := dialStreamAs(t, h, h.token)
	require.NotNil(t, c, "upgrade answered %d", status)
	c.Next(t, isReady)

	c.Sub(t, "projects", 0)
	var projects []string
	c.Next(t, func(f stream.Frame) bool {
		if f.T == stream.FrameEntity && f.Kind == string(stream.KindProject) {
			projects = append(projects, f.ID)
		}
		return isSynced("projects")(f)
	})
	c.Hold(t, "ws:"+imported.workspaceID)

	assert.Contains(t, projects, imported.projectID)
}

func TestRemoteStream_RefusesWithoutABearer(t *testing.T) {
	h := newPairedHarness(t)

	for _, token := range []string{"", "not-a-token"} {
		c, status := dialStreamAs(t, h, token)
		assert.Nil(t, c)
		assert.Equal(t, http.StatusUnauthorized, status, "token %q", token)
	}
}

// A host document is the local socket's alone, even for a paired client.
func TestRemoteStream_RefusesADocScope(t *testing.T) {
	h := newPairedHarness(t)
	c, _ := dialStreamAs(t, h, h.token)
	require.NotNil(t, c)
	scope := "doc:" + filepath.Join(t.TempDir(), "a.md")

	c.Sub(t, scope, 0)
	closed := c.Next(t, func(f stream.Frame) bool { return f.T == stream.FrameClosed && f.Scope == scope })

	assert.Equal(t, stream.ReasonRefused, closed.Reason)
	require.NotNil(t, closed.Problem)
	assert.Equal(t, "host_document_requires_local_transport", string(closed.Problem.Code))
}

func TestRemoteStream_RevokingTheClientClosesTheStream(t *testing.T) {
	h := newPairedHarness(t)
	c, _ := dialStreamAs(t, h, h.token)
	require.NotNil(t, c)
	c.Next(t, isReady)
	c.Hold(t, "projects")

	h.del("/v0/clients/"+h.clientID, nil, http.StatusOK, nil)

	for {
		var f stream.Frame
		if err := c.conn.ReadJSON(&f); err != nil {
			break
		}
	}
	_, status := dialStreamAs(t, h, h.token)
	assert.Equal(t, http.StatusUnauthorized, status, "a revoked token upgrades no more")
}

// Two daemons are two version spaces: each projects snapshot carries only its
// own rows, and restarting one changes only its epoch.
func TestTwoDaemons_IdsAndEpochsAreIndependent(t *testing.T) {
	local, remote := newHarness(t), newPairedHarness(t)
	onLocal := importWritableWorkspace(t, local)
	onRemote := importWritableWorkspace(t, remote.harness)
	projectsOf := func(c *streamClient) (string, []string) {
		epoch := c.Next(t, isReady).Epoch
		c.Sub(t, "projects", 0)
		var ids []string
		c.Next(t, func(f stream.Frame) bool {
			if f.T == stream.FrameEntity && f.Kind == string(stream.KindProject) {
				ids = append(ids, f.ID)
			}
			return isSynced("projects")(f)
		})
		return epoch, ids
	}
	remoteClient, _ := dialStreamAs(t, remote, remote.token)
	require.NotNil(t, remoteClient)

	localEpoch, localIDs := projectsOf(dialStream(t, local))
	remoteEpoch, remoteIDs := projectsOf(remoteClient)

	assert.Equal(t, []string{onLocal.projectID}, localIDs)
	assert.Equal(t, []string{onRemote.projectID}, remoteIDs)
	assert.NotEqual(t, localEpoch, remoteEpoch)
	local.shutdown()
	again := newHarnessAt(t, local.home)
	restarted, _ := projectsOf(dialStream(t, again))
	assert.NotEqual(t, localEpoch, restarted)
	stillRemote, _ := dialStreamAs(t, remote, remote.token)
	require.NotNil(t, stillRemote)
	assert.Equal(t, remoteEpoch, stillRemote.Next(t, isReady).Epoch, "the other daemon's epoch is untouched")
}
