package main

import "testing"

func TestScopedAgentPath(t *testing.T) {
	got := scopedAgentPath("p1", "r1", "w1", "/chats/c1/rename?source=agent")
	want := "/v0/projects/p1/repos/r1/workspaces/w1/agent/chats/c1/rename?source=agent"
	if got != want {
		t.Fatalf("scopedAgentPath = %q, want %q", got, want)
	}
}
