package exec

import (
	"context"
	"time"
)

// RunWithLockRetry exposes runWithLockRetry to the external test package.
func RunWithLockRetry(
	ctx context.Context,
	run func() Result,
) Result {
	return runWithLockRetry(ctx, run)
}

// IsIndexLockFailure exposes isIndexLockFailure to the external test package.
func IsIndexLockFailure(
	r Result,
) bool {
	return isIndexLockFailure(r)
}

// LockPathFromResult exposes lockPathFromResult to the external test package.
func LockPathFromResult(
	r Result,
) string {
	return lockPathFromResult(r)
}

// RemoveStaleLock exposes removeStaleLock to the external test package.
func RemoveStaleLock(
	lockPath string,
) bool {
	return removeStaleLock(lockPath)
}

// SetSleepForTest replaces the retry backoff sleeper and returns a restore func.
func SetSleepForTest(
	fn func(time.Duration),
) func() {
	prev := sleepFn
	sleepFn = fn
	return func() { sleepFn = prev }
}

// SetNowForTest replaces the stale-lock clock and returns a restore func.
func SetNowForTest(
	fn func() time.Time,
) func() {
	prev := nowFn
	nowFn = fn
	return func() { nowFn = prev }
}

// SetGitResolverForTest replaces the pair of functions that decide which binary
// a git invocation exec's, and returns a restore func.
func SetGitResolverForTest(
	bin func() string,
	recover func() bool,
) func() {
	prevBin, prevRecover := gitBin, recoverGit
	gitBin, recoverGit = bin, recover
	return func() { gitBin, recoverGit = prevBin, prevRecover }
}
