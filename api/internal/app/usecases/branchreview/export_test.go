package branchreview

import (
	"golang.org/x/sync/singleflight"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// SharedScope exposes sharedScope to the external test package. Unwrapping a
// flight result for one waiter is a pure function, so the property that matters
// about it — that waiters never alias one backing array — is provable without
// racing several callers onto a live flight.
func SharedScope(
	res singleflight.Result,
) (gitdomain.ReviewScope, error) {
	return sharedScope(res)
}
