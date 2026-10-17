package main

import (
	"path/filepath"
	"testing"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

func TestSeedRoot_RequiresCrowbarHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(home.EnvVar, "")
	_, err := seedRoot()
	if err == nil {
		t.Fatal("seedRoot() succeeded with CROWBAR_HOME unset; it must refuse to guess the user's real home")
	}
}

func TestSeedRoot_RefusesTheDefaultHomeSpelledOut(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv(home.EnvVar, filepath.Join(userHome, ".crowbar"))
	_, err := seedRoot()
	if err == nil {
		t.Fatal("seedRoot() succeeded with CROWBAR_HOME naming the default home; it must refuse it")
	}
}

func TestSeedRoot_UsesCrowbarHomeWhenSet(t *testing.T) {
	dev := t.TempDir()
	t.Setenv(home.EnvVar, dev)
	got, err := seedRoot()
	if err != nil {
		t.Fatalf("seedRoot() error: %v", err)
	}
	want := filepath.Join(dev, "seed")
	if got != want {
		t.Fatalf("seedRoot() = %q, want %q", got, want)
	}
}
