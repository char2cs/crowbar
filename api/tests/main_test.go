//go:build integration

package tests

import (
	"testing"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestMain routes this package through the shared harness entry point, which
// silences logs, sets gin test mode and — the reason this file exists — pins
// CROWBAR_HOME to a throwaway directory.
//
// Without it, anything here that resolves the crowbar home without an injected
// one wrote into the developer's real ~/.crowbar: the agent-chat spawn path did
// exactly that, and a live home accumulated an empty project directory per run.
func TestMain(m *testing.M) {
	kit.Main(m)
}
