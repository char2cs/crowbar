package runner

import (
	"os"
	"path/filepath"
	"testing"
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
