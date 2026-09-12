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
