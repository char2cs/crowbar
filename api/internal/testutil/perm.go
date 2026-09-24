// Package testutil holds helpers shared by tests across packages. Nothing in
// production code may import it.
package testutil

import (
	"os"
	"runtime"
	"testing"
)

// RequirePermissionEnforcement skips a test that provokes a failure through
// file permissions (a read-only dir, a 0o000 file) when the OS will not
// enforce them: on Windows, and when running as root (as in CI containers),
// where every chmod is bypassed and the expected error never happens.
func RequirePermissionEnforcement(tb testing.TB) {
	tb.Helper()
	if runtime.GOOS == "windows" {
		tb.Skip("unix permission semantics")
	}
	if os.Geteuid() == 0 {
		tb.Skip("running as root: file permissions are not enforced, so the failure cannot be provoked")
	}
}
