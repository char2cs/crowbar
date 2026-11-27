package exec

import (
	"context"
	"time"
)

func RunWithLockRetry(
	ctx context.Context,
	run func() Result,
) Result {
	return runWithLockRetry(ctx, run)
}

func IsIndexLockFailure(
	r Result,
) bool {
	return isIndexLockFailure(r)
}

func LockPathFromResult(
	r Result,
) string {
	return lockPathFromResult(r)
}

func RemoveStaleLock(
	lockPath string,
) bool {
	return removeStaleLock(lockPath)
}

func ClassifyTimeout(
	boundedErr error,
	parentErr error,
	r Result,
) Result {
	return classifyTimeout(boundedErr, parentErr, r)
}

func SetSleepForTest(
	fn func(time.Duration),
) func() {
	prev := sleepFn
	sleepFn = fn
	return func() { sleepFn = prev }
}

func SetNowForTest(
	fn func() time.Time,
) func() {
	prev := nowFn
	nowFn = fn
	return func() { nowFn = prev }
}

func SetGitResolverForTest(
	bin func() string,
	recover func() bool,
) func() {
	prevBin, prevRecover := gitBin, recoverGit
	gitBin, recoverGit = bin, recover
	return func() { gitBin, recoverGit = prevBin, prevRecover }
}
