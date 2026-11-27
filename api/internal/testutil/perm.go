package testutil

import (
	"os"
	"runtime"
	"testing"
)

func RequirePermissionEnforcement(tb testing.TB) {
	tb.Helper()
	if runtime.GOOS == "windows" {
		tb.Skip("unix permission semantics")
	}
	if os.Geteuid() == 0 {
		tb.Skip("running as root: file permissions are not enforced, so the failure cannot be provoked")
	}
}
