package exec

import (
	"context"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	lockRetryAttempts = 3
	lockRetryBackoff  = 150 * time.Millisecond
	staleLockMaxAge   = 10 * time.Second
)

var (
	sleepFn    = time.Sleep
	nowFn      = time.Now
	lockPathRe = regexp.MustCompile(`Unable to create '([^']+\.lock)'`)
)

func runWithLockRetry(
	ctx context.Context,
	run func() Result,
) Result {
	var r Result
	for attempt := 0; attempt < lockRetryAttempts; attempt++ {
		if attempt > 0 {
			sleepFn(lockRetryBackoff)
		}
		r = run()
		if !isIndexLockFailure(r) || ctx.Err() != nil {
			return r
		}
	}
	if !removeStaleLock(lockPathFromResult(r)) {
		return r
	}
	return run()
}

func isIndexLockFailure(
	r Result,
) bool {
	if r.ExitCode == 0 {
		return false
	}
	return strings.Contains(r.Stderr, "index.lock': File exists")
}

func lockPathFromResult(
	r Result,
) string {
	m := lockPathRe.FindStringSubmatch(r.Stderr)
	if m == nil {
		return ""
	}
	return m[1]
}

func removeStaleLock(
	lockPath string,
) bool {
	if lockPath == "" {
		return false
	}
	info, err := os.Stat(lockPath)
	if err != nil {
		return false
	}
	if nowFn().Sub(info.ModTime()) < staleLockMaxAge {
		return false
	}
	return os.Remove(lockPath) == nil
}
