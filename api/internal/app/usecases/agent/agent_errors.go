package agent

import (
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
)

// ErrProviderDisabled is returned when a request names an agent provider the
// user has switched OFF in the global preference table.
//
// Disabled is a decision the preference table records and ResolveProviders
// reports, but reporting is not enforcing: a spawn or a provider switch names a
// provider id directly and never passes through the list that Enabled flag
// decorates, so a stale tab, a second window, or the command line would
// otherwise launch a provider the user turned off.
//
// It wraps apperr.ErrInvalidArgument, so handlers answer 400 through the
// existing sentinel mapping with no new case.
var ErrProviderDisabled = fmt.Errorf("agent: provider disabled: %w", apperr.ErrInvalidArgument)
