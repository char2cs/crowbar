package models

import "testing"

func TestScopeFlags_IncludesHome(t *testing.T) {
	c := TemplateCtx{ProjectID: "p1", WorkspaceID: "w1", CrowbarHome: "/Users/x/.crowbar"}
	got := c.ScopeFlags()
	want := "--project=p1 --workspace=w1 --home=/Users/x/.crowbar"
	if got != want {
		t.Fatalf("ScopeFlags() = %q, want %q", got, want)
	}
}

func TestScopeFlags_OmitsHomeWhenEmpty(t *testing.T) {
	c := TemplateCtx{ProjectID: "p1", WorkspaceID: "w1"}
	got := c.ScopeFlags()
	want := "--project=p1 --workspace=w1"
	if got != want {
		t.Fatalf("ScopeFlags() = %q, want %q", got, want)
	}
}

func TestReplacer_ExpandsCrowbarHome(t *testing.T) {
	c := TemplateCtx{CrowbarHome: "/tmp/home"}
	got := c.Replacer().Replace("{crowbar_home}")
	if got != "/tmp/home" {
		t.Fatalf("{crowbar_home} expanded to %q, want %q", got, "/tmp/home")
	}
}

// Hook commands run through the vendor CLI's shell: a home under a directory
// with a space must still reach the relay as one word, or no hook ever fires.
func TestHookCommandValues_SurviveAShell(t *testing.T) {
	c := TemplateCtx{
		ProjectID: "p1", WorkspaceID: "w1",
		CrowbarHome: "/Users/Jo Smith/.crowbar", CrowbarHook: "/Users/Jo Smith/.crowbar/bin/crowbar",
	}
	got := c.Replacer().Replace("{crowbar_hook} hook turn_stop {scope_flags}")
	want := "'/Users/Jo Smith/.crowbar/bin/crowbar' hook turn_stop --project=p1 --workspace=w1 --home='/Users/Jo Smith/.crowbar'"
	if got != want {
		t.Fatalf("hook command = %q, want %q", got, want)
	}
	if raw := c.Replacer().Replace("{crowbar}"); raw != c.CrowbarHook {
		t.Fatalf("{crowbar} is an argv/config value, not shell: got %q", raw)
	}
}
