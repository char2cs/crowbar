//go:build windows

package session

import "os"

// terminateSignal has no clean-exit primitive to fall back on: Windows has no
// POSIX signal delivery, and os.Process.Signal only supports os.Kill there
// (anything else returns "not supported by windows"). Terminate degrades to
// an immediate hard kill on this platform — callers should not rely on
// transcript-flush semantics for graceful terminate on Windows.
func terminateSignal(proc *os.Process) error {
	return proc.Kill()
}
