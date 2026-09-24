//go:build integration

package tests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// permissionStubProviderDescriptorYAML is liveStubProviderDescriptorYAML's cat
// (see agent_working_overlay_test.go) plus a permission_levels block — two
// declared levels, restart_tui, mirroring claude.yaml/codex.yaml's own real
// blocks. Unlike switchableStubProviderDescriptorYAML's model/effort apply
// steps (which never actually fire in that suite — Model/Effort default to ""
// and Steps() only renders their pass_arg when non-empty), a permission level
// is NEVER empty — every chat is always seeded with a real one — so these
// apply steps run on EVERY spawn. Each level's flag must therefore be one
// `cat` genuinely accepts (BSD cat has no long-option support at all, and an
// unrecognised flag exits it instantly with 424 before this test's own
// assertions ever run — confirmed the hard way writing this test).
const permissionStubProviderDescriptorYAML = `id: permstub
spawn:
  cmd: "cat"
  interactive_required: true
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

// TestRegression_PermissionLevelFollowsTheGlobalDefaultOnAnInheritedChatsNextSpawn
// is the real reported bug: a chat that has NEVER had its own explicit
// guarded/trusted/full-auto pick (SetChatPermissionLevel was never called on
// it) keeps spawning under whatever the global default was AT CREATION TIME
// forever, even after the user changes the dial in Settings — because
// ChatSelection (internal/conversation/selection.go) read the chat's own
// frozen, seeded-at-mint field unconditionally, never the live global
// default, for any chat whose field was non-empty. Confirmed live: the user
// flips Settings to full-auto and every EXISTING chat keeps answering from
// whatever it was minted under.
//
// A brand-new chat's first-ever spawn always looked correct (it seeds FROM
// the current default), which is exactly why the empirical probe in the
// original investigation missed this: it only ever spawned fresh chats.
//
// This drives the real HTTP surface end to end: PUT the global default,
// create a chat (spawns under it), PUT a NEW default, force a respawn via
// the real switch endpoint (same provider — a restart, not a provider
// change), and read the runner's own recorded LaunchPermissionLevel — the
// exact fact SelectionSteps/PermissionVars render the CLI argv from (see
// runner/spawn.go's recordRunner and resolveSelectionForSpawn), the same
// proxy this suite's own TestRegression_SwitchingProvidersDropsAModelThe...
// uses for LaunchModel.
func TestRegression_PermissionLevelFollowsTheGlobalDefaultOnAnInheritedChatsNextSpawn(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "permstub", permissionStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)

	h.put("/v0/settings/chat/permission-level", map[string]string{"level": "guarded"}, nil)

	chatID, _ := createStubChat(t, h, imported, "permstub")

	before, err := h.app.Repositories.AgentRunner.LiveRunnerForChat(context.Background(), chatID)
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

	after, err := h.app.Repositories.AgentRunner.LiveRunnerForChat(context.Background(), chatID)
	require.NoError(t, err, "the restart must leave a live runner on the chat")
	assert.Equal(t, "full-auto", after.LaunchPermissionLevel,
		"a chat that was never explicitly pinned (SetChatPermissionLevel) must follow the "+
			"global default at its NEXT spawn, not stay frozen at whatever the default was "+
			"when it was minted")
}

// TestRegression_AnExplicitChatPickStillWinsOverALaterGlobalDefaultChange is
// the fix's OTHER half: SetChatPermissionLevel must still pin a chat against
// the dial once a human has actually chosen for it — the inherit-by-default
// behaviour above must never silently override an explicit choice.
func TestRegression_AnExplicitChatPickStillWinsOverALaterGlobalDefaultChange(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "permstub", permissionStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)

	h.put("/v0/settings/chat/permission-level", map[string]string{"level": "guarded"}, nil)
	chatID, _ := createStubChat(t, h, imported, "permstub")

	h.put(repoBase(imported)+"/chats/"+chatID+"/permission-level",
		map[string]string{"level": "full-auto"}, nil)
	h.Quiesce()

	// The dial now disagrees with the explicit pick in BOTH directions —
	// changed away from what was picked, so an unguarded "always follow the
	// default" fix would just as surely have broken this the other way.
	h.put("/v0/settings/chat/permission-level", map[string]string{"level": "guarded"}, nil)

	var result struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats/"+chatID+"/switch",
		map[string]string{"provider": "permstub"}, http.StatusOK, &result)
	h.QuiesceReactors()

	after, err := h.app.Repositories.AgentRunner.LiveRunnerForChat(context.Background(), chatID)
	require.NoError(t, err, "the restart must leave a live runner on the chat")
	assert.Equal(t, "full-auto", after.LaunchPermissionLevel,
		"an explicit per-chat pick must keep winning over the global default, even after "+
			"the default changes again")
}
