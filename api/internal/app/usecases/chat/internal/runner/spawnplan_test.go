package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/core/hookserver"
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
