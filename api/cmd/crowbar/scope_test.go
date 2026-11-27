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

func TestScope_ASetenvFailureIsReturned(t *testing.T) {
	t.Setenv("CROWBAR_HOME", "/already/set")
	if err := applyHomeOverride("/tmp/x\x00y"); err == nil {
		t.Fatal("applyHomeOverride with a NUL byte: want an error")
	}
}

func TestScopedAgentPath(t *testing.T) {
	got := scopedAgentPath("p1", "r1", "w1", "/c1/rename?source=agent")
	want := "/v0/projects/p1/repos/r1/chats/c1/rename?source=agent"
	if got != want {
		t.Fatalf("scopedAgentPath = %q, want %q", got, want)
	}
}

func TestScopedAgentPath_HomeWorkspaceHasNoRepo(t *testing.T) {
	got := scopedAgentPath("p1", "", "home-ws", "/runners/s1/mcp")
	want := "/v0/projects/p1/home/chats/runners/s1/mcp"
	if got != want {
		t.Fatalf("scopedAgentPath(home) = %q, want %q", got, want)
	}
}

func TestScopedAgentPath_HomeSuffixesCompose(t *testing.T) {
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
