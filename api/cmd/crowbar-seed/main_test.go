package main

import (
	"path/filepath"
	"testing"
)

func TestSeedRoot_RequiresCrowbarHome(t *testing.T) {
	t.Setenv("CROWBAR_HOME", "")
	_, err := seedRoot()
	if err == nil {
		t.Fatal("seedRoot() succeeded with CROWBAR_HOME unset; it must refuse to guess the user's real home")
	}
}

func TestSeedRoot_UsesCrowbarHomeWhenSet(t *testing.T) {
	t.Setenv("CROWBAR_HOME", "/tmp/some-dev-home")
	got, err := seedRoot()
	if err != nil {
		t.Fatalf("seedRoot() error: %v", err)
	}
	want := filepath.Join("/tmp/some-dev-home", "seed")
	if got != want {
		t.Fatalf("seedRoot() = %q, want %q", got, want)
	}
}
