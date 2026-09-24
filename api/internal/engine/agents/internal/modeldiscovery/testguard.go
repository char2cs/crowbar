package modeldiscovery

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// liveProbeEnvVar re-enables guardedProbe's real fork from inside a go test
// binary — an escape hatch for a test that deliberately wants a live probe,
// mirroring how other packages gate a real-CLI fork behind an env var.
const liveProbeEnvVar = "CROWBAR_MODEL_DISCOVERY_LIVE"

// errProbingDisabledUnderTest is guardedProbe's refusal — handled identically
// to any other probe failure by attemptRefresh, so it never blanks an
// already-resolved catalogue.
var errProbingDisabledUnderTest = errors.New("modeldiscovery: live probing disabled under go test; set " +
	liveProbeEnvVar + "=1 to override")

// guardedProbe is NewCache's default probeFunc. A descriptor's own command
// name is resolved by exec.Executable against PATH, then against
// well-known install directories a real machine may have a vendor CLI
// sitting in — a fallback that exists for production, not for a test
// binary, but applies unconditionally either way. Left unguarded, any test
// that builds a Cache without swapping in its own probeFunc (every caller
// outside this package) would fork whatever binary that fallback finds,
// for real: slow, non-hermetic, dependent on what happens to be installed
// on the machine running the test — and racing a real disk write against
// that same test's own teardown is the exact defect this guard exists for.
//
// testing.Testing() is true only inside a binary built by `go test`, so this
// can never fire in a production build.
func guardedProbe(
	ctx context.Context, d *spec.Descriptor, opts models.ProbeOptions, acquire exec.Acquire,
) ([]Model, error) {
	if testing.Testing() && os.Getenv(liveProbeEnvVar) == "" {
		return nil, errProbingDisabledUnderTest
	}
	return Probe(ctx, d, opts, acquire)
}
