package main

import "testing"

func TestScopedAgentPath(t *testing.T) {
	got := scopedAgentPath("p1", "r1", "w1", "/c1/rename?source=agent")
	want := "/v0/projects/p1/repos/r1/workspaces/w1/chats/c1/rename?source=agent"
	if got != want {
		t.Fatalf("scopedAgentPath = %q, want %q", got, want)
	}
}

func TestScopedAgentPath_HomeWorkspaceHasNoRepo(t *testing.T) {
	// A project-home workspace resolves an EMPTY repo id (WorktreeDir returns ""
	// for the project-level home — see agentWorkspaceReader.AgentChatsDir's doc).
	// The callback must target the home-group mount that home.Register serves
	// (added in commit 1), NOT /repos//workspaces/.../chats, which 404s. This is
	// the project-home half of the spec's CRITICAL "chats work for ALL workspace
	// kinds" requirement.
	got := scopedAgentPath("p1", "", "home-ws", "/hooks")
	want := "/v0/projects/p1/home/chats/hooks"
	if got != want {
		t.Fatalf("scopedAgentPath(home) = %q, want %q", got, want)
	}
}

func TestScopedAgentPath_HomeSuffixesCompose(t *testing.T) {
	// The home branch must compose with every callback suffix — the hook
	// (hook.go), the agent rename with its ?source=agent query (chat.go), and the
	// handoff dump (handoff.go) — each landing on the matching home-group route.
	cases := map[string]string{
		"/hooks":                  "/v0/projects/p1/home/chats/hooks",
		"/c1/rename?source=agent": "/v0/projects/p1/home/chats/c1/rename?source=agent",
		"/c1/handoff":             "/v0/projects/p1/home/chats/c1/handoff",
	}
	for suffix, want := range cases {
		if got := scopedAgentPath("p1", "", "home-ws", suffix); got != want {
			t.Fatalf("scopedAgentPath(home, %q) = %q, want %q", suffix, got, want)
		}
	}
}
