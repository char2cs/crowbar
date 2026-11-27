//go:build integration

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const permissionStubProviderDescriptorYAML = `id: permstub
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  user_prompt:
    in: user_prompt
    map:
      message: prompt
  turn_stop:
    in: turn_stop
    map:
      session_id: session_id
      message: last_assistant_message
permission_levels:
  strategy: restart_tui
  levels:
    guarded:
      apply:
        - pass_arg: { arg: "-u" }
    full-auto:
      apply:
        - pass_arg: { arg: "-n" }
runtime:
  transport: hooks
  hooks:
    format: json
`

func TestRegression_PermissionLevelFollowsTheGlobalDefaultOnAnInheritedChatsNextSpawn(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "permstub", permissionStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)

	h.put("/v0/settings/chat/permission-level", map[string]string{"level": "guarded"}, nil)

	chatID, _ := createStubChat(t, h, imported, "permstub")

	before, err := h.app.Usecases.AgentRunner.LiveRunnerForChat(context.Background(), chatID)
	require.NoError(t, err, "the freshly spawned chat must have a live runner")
	require.Equal(t, "guarded", before.LaunchPermissionLevel,
		"precondition: spawned under the global default in effect at creation")

	h.put("/v0/settings/chat/permission-level", map[string]string{"level": "full-auto"}, nil)

	var result struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats/"+chatID+"/switch",
		map[string]string{"provider": "permstub"}, http.StatusOK, &result)
	h.QuiesceReactors()

	after, err := h.app.Usecases.AgentRunner.LiveRunnerForChat(context.Background(), chatID)
	require.NoError(t, err, "the restart must leave a live runner on the chat")
	assert.Equal(t, "full-auto", after.LaunchPermissionLevel,
		"a chat that was never explicitly pinned (SetChatPermissionLevel) must follow the "+
			"global default at its NEXT spawn, not stay frozen at whatever the default was "+
			"when it was minted")
}

func TestRegression_AnExplicitChatPickStillWinsOverALaterGlobalDefaultChange(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "permstub", permissionStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)

	h.put("/v0/settings/chat/permission-level", map[string]string{"level": "guarded"}, nil)
	chatID, _ := createStubChat(t, h, imported, "permstub")

	h.put(repoBase(imported)+"/chats/"+chatID+"/permission-level",
		map[string]string{"level": "full-auto"}, nil)
	h.Quiesce()

	h.put("/v0/settings/chat/permission-level", map[string]string{"level": "guarded"}, nil)

	var result struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats/"+chatID+"/switch",
		map[string]string{"provider": "permstub"}, http.StatusOK, &result)
	h.QuiesceReactors()

	after, err := h.app.Usecases.AgentRunner.LiveRunnerForChat(context.Background(), chatID)
	require.NoError(t, err, "the restart must leave a live runner on the chat")
	assert.Equal(t, "full-auto", after.LaunchPermissionLevel,
		"an explicit per-chat pick must keep winning over the global default, even after "+
			"the default changes again")
}
