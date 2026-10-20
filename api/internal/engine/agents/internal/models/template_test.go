package models

import (
	"strings"
	"testing"
)

func TestScopeFlags_IncludesHome(t *testing.T) {
	c := TemplateCtx{ProjectID: "p1", WorkspaceID: "w1", CrowbarHome: "/Users/x/.crowbar"}
	got := c.ScopeFlags()
	want := "--project=p1 --workspace=w1 --home=/Users/x/.crowbar"
	if got != want {
		t.Fatalf("ScopeFlags() = %q, want %q", got, want)
	}
}

// A callback dials the socket the daemon bound, not one it derives from --home.
func TestScopeFlags_HandsCallbacksTheDaemonSocket(t *testing.T) {
	c := TemplateCtx{
		ProjectID: "p1", WorkspaceID: "w1", CrowbarHome: "/Users/x/.crowbar",
		DaemonSocket: "/tmp/crowbar-abc.sock",
	}
	got := c.ScopeFlags()
	want := "--project=p1 --workspace=w1 --home=/Users/x/.crowbar --host=unix:///tmp/crowbar-abc.sock"
	if got != want {
		t.Fatalf("ScopeFlags() = %q, want %q", got, want)
	}
}

func TestScopeFlags_QuotesADaemonSocketWithASpace(t *testing.T) {
	c := TemplateCtx{ProjectID: "p1", WorkspaceID: "w1", DaemonSocket: "/tmp/a b.sock"}
	got := c.ScopeFlags()
	want := "--project=p1 --workspace=w1 --host='unix:///tmp/a b.sock'"
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

// A hook posts to the runner-keyed route under the same project/repo scope the
// relay's path was built from: a repo-less project home takes the home branch.
func TestHookURL_ScopesToTheRunnerLikeTheRelayDid(t *testing.T) {
	base := TemplateCtx{
		HookBase: "http://127.0.0.1:4242", ProjectID: "p1", Segid: "seg-1", Provider: "claude",
	}

	repo := base
	repo.RepoID = "r1"
	if got, want := repo.HookURL(), "http://127.0.0.1:4242/v0/projects/p1/repos/r1/chats/runners/seg-1/hooks/claude"; got != want {
		t.Fatalf("HookURL() = %q, want %q", got, want)
	}
	if got, want := base.HookURL(), "http://127.0.0.1:4242/v0/projects/p1/home/chats/runners/seg-1/hooks/claude"; got != want {
		t.Fatalf("HookURL() for a project home = %q, want %q", got, want)
	}
	if got := (TemplateCtx{ProjectID: "p1"}).HookURL(); got != "" {
		t.Fatalf("HookURL() with no hook server = %q, want empty", got)
	}
}

func TestReplacer_ExpandsTheHookURL(t *testing.T) {
	c := TemplateCtx{HookBase: "http://127.0.0.1:1", ProjectID: "p", RepoID: "r", Segid: "s", Provider: "codex"}
	got := c.Replacer().Replace("{hook_url}/tool_pre")
	want := "http://127.0.0.1:1/v0/projects/p/repos/r/chats/runners/s/hooks/codex/tool_pre"
	if got != want {
		t.Fatalf("{hook_url} expanded to %q, want %q", got, want)
	}
}

// A CLI that can only run a command posts through curl, reading its credential from a
// file under the runner's own temp dir so the token never sits in argv for `ps`.
func TestHookCurl_ReadsItsCredentialFromAFileNotArgv(t *testing.T) {
	c := TemplateCtx{Tmp: "/var/folders/Jo Smith/T/run-1", RunnerToken: "secret-token"}
	got := c.Replacer().Replace("{hook_curl}")
	want := "curl --fail --silent --show-error --config '/var/folders/Jo Smith/T/run-1/hook.curl'"
	if got != want {
		t.Fatalf("{hook_curl} expanded to %q, want %q", got, want)
	}
	if strings.Contains(got, "secret-token") {
		t.Fatal("the runner token must not appear on the command line")
	}
}
