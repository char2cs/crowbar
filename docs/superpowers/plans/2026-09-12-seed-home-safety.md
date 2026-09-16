# crowbar-seed Home Safety Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `crowbar-seed` must refuse to run against a user's real `~/.crowbar` — it is dev tooling that seeds throwaway state, and today it silently falls back to the real home whenever `CROWBAR_HOME` is unset.

**Architecture:** `seedRoot()` (`api/cmd/crowbar-seed/main.go`) currently does: env override, else `os.UserHomeDir() + "/.crowbar"`. Remove the fallback branch entirely — `CROWBAR_HOME` becomes required, and its absence is a clear, immediate error instead of silent production writes. `make seed`/`make seed-chat` already export it (`Makefile:11`), so the only invocations this can break are ones that bypass `make` and run the binary directly — exactly the invocations this plan exists to stop.

**Tech Stack:** Go.

**Spec:** This plan's own Goal/Architecture above — informal findings from a live diagnostic session. Confirmed live in the session: `~/.crowbar/projects/p2/` (an empty, DB-orphaned directory matching `crowbar-seed`'s hardcoded `seedProjectName`/`"Crowbar Seed"` fixture) exists on the production home, proving this fallback has already fired for real at least once.

## Global Constraints

- Only the actual installed/production daemon binary (`api/cmd/crowbar`, run by the desktop app with `CROWBAR_HOME` deliberately left unset — see `desktop/src-tauri/src/sidecar/mod.rs:210-212`) is allowed to resolve `~/.crowbar` as a default. Every dev-only tool must require the override explicitly and fail loudly without it.

---

### Task 1: `seedRoot()` requires `CROWBAR_HOME`

**Files:**
- Modify: `api/cmd/crowbar-seed/main.go`
- Test: `api/cmd/crowbar-seed/main_test.go` (new — no test file exists for this package's `main.go` today)

**Interfaces:**
- Produces: `seedRoot() (string, error)` returns a clear error when `CROWBAR_HOME` is unset or empty; unchanged behavior when it is set.

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./api/cmd/crowbar-seed/... -run TestSeedRoot -v`
Expected: `TestSeedRoot_RequiresCrowbarHome` FAILS (current code falls back to `os.UserHomeDir()` and succeeds); `TestSeedRoot_UsesCrowbarHomeWhenSet` already passes.

- [ ] **Step 3: Write minimal implementation**

Replace `seedRoot()` in `main.go`:

```go
// seedRoot puts the throwaway repo under the crowbar home CROWBAR_HOME names.
// It is disposable state that already lives outside the source tree, and
// under a dev CROWBAR_HOME it is thrown away with the rest of the dev
// instance.
//
// CROWBAR_HOME is REQUIRED, not defaulted: this is dev/test tooling that
// mints throwaway projects and repos, and a silent fallback to the user's
// real ~/.crowbar (formerly via os.UserHomeDir()) is exactly how a stray
// "Crowbar Seed" project ends up seeded into production the one time someone
// runs this binary directly instead of through `make seed`, which is the
// only thing that reliably exports CROWBAR_HOME (Makefile:11).
func seedRoot() (string, error) {
	home := os.Getenv(metadata.HomeEnvVar)
	if home == "" {
		return "", fmt.Errorf(
			"seed: CROWBAR_HOME is not set — run this through `make seed` " +
				"(or export CROWBAR_HOME yourself); crowbar-seed refuses to guess " +
				"and never writes to a real ~/.crowbar",
		)
	}
	return filepath.Join(home, "seed"), nil
}
```

Remove the now-unused `os.UserHomeDir()` call; if `os` has no other use in the file, drop the import (it is still used elsewhere in `main.go` for `os.Stdout`/`os.Exit`, so it will remain needed).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./api/cmd/crowbar-seed/... -v`
Expected: PASS, including the rest of the package's existing tests (`fixture_test.go`, etc. — this change touches only `seedRoot`).

- [ ] **Step 5: Commit**

```bash
git add api/cmd/crowbar-seed/main.go api/cmd/crowbar-seed/main_test.go
git commit -m "fix(crowbar-seed): require CROWBAR_HOME instead of falling back to the real home"
```

---

## Self-Review Notes

- **Spec coverage:** the one fallback identified (`seedRoot`) is the only place in `crowbar-seed` that resolves a home path (confirmed via `grep -n "CrowbarHome\|homeDir\|HomeDir\|os.UserHomeDir\|CROWBAR_HOME" api/cmd/crowbar-seed/*.go` during diagnosis) — no other task is needed for this binary.
- **Out of scope:** cleaning up the `~/.crowbar/projects/p2/` directory this bug already left behind — that is a one-time manual cleanup (or folds into a future reconciliation sweep, not yet planned — see the open item in `2026-09-12-crowbar-home-propagation.md`'s Self-Review Notes), not part of this code fix.
