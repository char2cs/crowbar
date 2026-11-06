package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/core/hookserver"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestRenderSpawnContext_CarriesCrowbarHome(t *testing.T) {
	rs := &Runners{}
	in := spawnContext{
		crowbarHome: "/tmp/some-dev-home",
		projectID:   "p1",
		workspaceID: "w1",
		runnerID:    "run1",
		providerID:  "claude",
	}
	tctx, _ := rs.renderSpawnContext(in)
	if tctx.CrowbarHome != "/tmp/some-dev-home" {
		t.Fatalf("TemplateCtx.CrowbarHome = %q, want %q", tctx.CrowbarHome, "/tmp/some-dev-home")
	}
}

// A forked terminal view renders the level through permission_levels.<level>.apply.
func TestRenderSpawnContext_CarriesThePermissionLevel(t *testing.T) {
	tctx, _ := (&Runners{}).renderSpawnContext(spawnContext{
		crowbarHome: "/tmp/h", runnerID: "run1",
		selection: engineagents.Selection{PermissionLevel: "trusted"},
	})

	if tctx.PermissionLevel != "trusted" {
		t.Fatalf("TemplateCtx.PermissionLevel = %q, want the chat's level", tctx.PermissionLevel)
	}
}

// Callbacks are handed the address the daemon bound; they never derive one.
func TestRenderSpawnContext_CarriesTheDaemonAddress(t *testing.T) {
	rs := &Runners{daemonHost: "tcp://127.0.0.1:3737"}

	tctx, _ := rs.renderSpawnContext(spawnContext{crowbarHome: "/tmp/h", runnerID: "run1"})

	if tctx.DaemonHost != "tcp://127.0.0.1:3737" {
		t.Fatalf("TemplateCtx.DaemonHost = %q, want the daemon's address", tctx.DaemonHost)
	}
}

// A provider keys its folder trust on the canonical path, so a symlinked
// worktree must reach the CLI resolved or its trust prompt parks the TUI.
func TestRenderSpawnContext_ResolvesASymlinkedWorktree(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	want, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	tctx, _ := (&Runners{}).renderSpawnContext(spawnContext{worktree: link})
	if tctx.Cwd != want {
		t.Fatalf("TemplateCtx.Cwd = %q, want %q", tctx.Cwd, want)
	}
}

// A CLI's hooks are pointed at the hook server the daemon published under the home
// this spawn resolved against.
func TestRenderSpawnContext_PointsHooksAtThePublishedHookServer(t *testing.T) {
	home := t.TempDir()
	if err := hookserver.Publish(home, "http://127.0.0.1:4242"); err != nil {
		t.Fatal(err)
	}

	tctx, _ := (&Runners{}).renderSpawnContext(spawnContext{
		crowbarHome: home, projectID: "p1", repoID: "r1", runnerID: "run1", providerID: "claude",
	})

	if tctx.HookBase != "http://127.0.0.1:4242" {
		t.Fatalf("TemplateCtx.HookBase = %q, want the published hook server", tctx.HookBase)
	}
}

func TestRenderSpawnContext_NoPublishedHookServerLeavesHooksUnpointed(t *testing.T) {
	tctx, _ := (&Runners{}).renderSpawnContext(spawnContext{crowbarHome: t.TempDir(), runnerID: "run1"})

	if tctx.HookBase != "" || tctx.HookURL() != "" {
		t.Fatalf("HookBase = %q, HookURL = %q, want both empty", tctx.HookBase, tctx.HookURL())
	}
}

// The binary the descriptors run for the MCP bridge is the one the daemon runs.
func TestRenderSpawnContext_RunsTheInjectedCrowbarBin(t *testing.T) {
	got, _ := (&Runners{crowbarBin: "/opt/crowbar"}).renderSpawnContext(spawnContext{crowbarHome: "/tmp/h", runnerID: "run1"})

	if got.CrowbarBin != "/opt/crowbar" {
		t.Fatalf("TemplateCtx.CrowbarBin = %q, want the injected binary", got.CrowbarBin)
	}
}

// The preamble leads; a missing half (a blanked capabilities_instruction, a chat
// with no conversation yet) leaves no stray blank line a model reads as a section.
func TestComposeContext_PreambleLeadsAndAbsentHalvesLeaveNoGap(t *testing.T) {
	cases := []struct{ name, preamble, conversation, want string }{
		{"a fresh chat has only the preamble", "PREAMBLE", "", "PREAMBLE"},
		{"a chat taking a handoff, with the preamble leading", "PREAMBLE", "HANDOFF", "PREAMBLE\n\nHANDOFF"},
		{"capabilities_instruction blanked in the user's own config.yaml", "", "HANDOFF", "HANDOFF"},
		{"nothing to say", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := composeContext(tc.preamble, tc.conversation); got != tc.want {
				t.Fatalf("composeContext = %q, want %q", got, tc.want)
			}
		})
	}
}
