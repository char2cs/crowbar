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

// runWithLockRetry runs the git command and, when it fails because another
// process holds .git/index.lock, retries up to lockRetryAttempts times with a
// short backoff. Other git processes (user terminals, shell prompt helpers)
// legitimately hold the lock for brief windows, so a failed first attempt is
// not an error yet.
//
// If every attempt fails and the lock file is older than staleLockMaxAge, the
// lock is removed once and the command retried one final time. The staleness
// check is age-only (no liveness probe of the owning process): a real git
// operation finishes in well under ten seconds, so a lock that old is almost
// certainly debris from a killed process. The tradeoff is accepted because a
// genuinely live >10s index lock is far rarer than a stale one wedging every
// mutation, and losing that race only fails one git command loudly.
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
