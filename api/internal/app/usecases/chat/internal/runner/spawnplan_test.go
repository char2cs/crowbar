package runner

import "testing"

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
