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

// liveProbeEnvVar re-enables guardedProbe's real fork inside a go test binary,
// for a test that deliberately wants a live probe.
const liveProbeEnvVar = "CROWBAR_MODEL_DISCOVERY_LIVE"

// errProbingDisabledUnderTest is guardedProbe's refusal; attemptRefresh treats it
// like any probe failure, so it never blanks a resolved catalogue.
var errProbingDisabledUnderTest = errors.New("modeldiscovery: live probing disabled under go test; set " +
	liveProbeEnvVar + "=1 to override")

// guardedProbe is NewCache's default probeFunc. exec.Executable falls back to
// well-known install dirs, so an unguarded test Cache would fork whatever real CLI
// the machine has; testing.Testing() keeps this a no-op in production builds.
func guardedProbe(
	ctx context.Context, d *spec.Descriptor, opts models.ProbeOptions, acquire exec.Acquire,
) ([]Model, error) {
	if testing.Testing() && os.Getenv(liveProbeEnvVar) == "" {
		return nil, errProbingDisabledUnderTest
	}
	return Probe(ctx, d, opts, acquire)
}
