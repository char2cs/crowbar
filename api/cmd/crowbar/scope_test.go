package main

import (
	"os"
	"testing"
)

func TestApplyHomeOverride_SetsEnvWhenNonEmpty(t *testing.T) {
	t.Setenv("CROWBAR_HOME", "")
	if err := applyHomeOverride("/tmp/x"); err != nil {
		t.Fatalf("applyHomeOverride: %v", err)
	}
	if got := os.Getenv("CROWBAR_HOME"); got != "/tmp/x" {
		t.Fatalf("CROWBAR_HOME = %q, want %q", got, "/tmp/x")
	}
}

func TestApplyHomeOverride_LeavesEnvAloneWhenEmpty(t *testing.T) {
	t.Setenv("CROWBAR_HOME", "/already/set")
	if err := applyHomeOverride(""); err != nil {
		t.Fatalf("applyHomeOverride: %v", err)
	}
	if got := os.Getenv("CROWBAR_HOME"); got != "/already/set" {
		t.Fatalf("CROWBAR_HOME = %q, want unchanged %q", got, "/already/set")
	}
}

// A home the process cannot take must stop the callback: running on would
// resolve whatever home the environment happens to carry.
func TestScope_ASetenvFailureIsReturned(t *testing.T) {
	t.Setenv("CROWBAR_HOME", "/already/set")
	if err := applyHomeOverride("/tmp/x\x00y"); err == nil {
		t.Fatal("applyHomeOverride with a NUL byte: want an error")
	}
}

// TestScopedAgentPath proves the repo branch no longer carries a workspace
// segment (Task 17: chat routes moved off .../workspaces/:wsId), and that the
// (now-unused for path purposes) workspace argument is still accepted.
func TestScopedAgentPath(t *testing.T) {
	got := scopedAgentPath("p1", "r1", "w1", "/c1/rename?source=agent")
	want := "/v0/projects/p1/repos/r1/chats/c1/rename?source=agent"
	if got != want {
		t.Fatalf("scopedAgentPath = %q, want %q", got, want)
	}
}

func TestScopedAgentPath_HomeWorkspaceHasNoRepo(t *testing.T) {
	// A project-home workspace resolves an EMPTY repo id (WorktreeDir returns ""
	// for the project-level home — see agentWorkspaceReader.AgentChatsDir's doc).
	// The callback must target the home-group mount that home.Register serves
	// (added in commit 1), NOT /repos//chats, which 404s. This is the
	// project-home half of the spec's CRITICAL "chats work for ALL workspace
	// kinds" requirement.
	got := scopedAgentPath("p1", "", "home-ws", "/runners/s1/mcp")
	want := "/v0/projects/p1/home/chats/runners/s1/mcp"
	if got != want {
		t.Fatalf("scopedAgentPath(home) = %q, want %q", got, want)
	}
}

func TestScopedAgentPath_HomeSuffixesCompose(t *testing.T) {
	// The home branch must compose with every callback suffix — the MCP relay
	// (mcp.go), the agent rename with its ?source=agent query, and the handoff
	// dump (handoff.go) — each landing on the matching home-group route.
	cases := map[string]string{
		"/runners/s1/mcp":         "/v0/projects/p1/home/chats/runners/s1/mcp",
		"/c1/rename?source=agent": "/v0/projects/p1/home/chats/c1/rename?source=agent",
		"/c1/handoff":             "/v0/projects/p1/home/chats/c1/handoff",
	}
	for suffix, want := range cases {
		if got := scopedAgentPath("p1", "", "home-ws", suffix); got != want {
			t.Fatalf("scopedAgentPath(home, %q) = %q, want %q", suffix, got, want)
		}
	}
}
