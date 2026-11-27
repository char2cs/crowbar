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

const liveProbeEnvVar = "CROWBAR_MODEL_DISCOVERY_LIVE"

var errProbingDisabledUnderTest = errors.New("modeldiscovery: live probing disabled under go test; set " +
	liveProbeEnvVar + "=1 to override")

func guardedProbe(
	ctx context.Context, d *spec.Descriptor, opts models.ProbeOptions, acquire exec.Acquire,
) ([]Model, error) {
	if testing.Testing() && os.Getenv(liveProbeEnvVar) == "" {
		return nil, errProbingDisabledUnderTest
	}
	return Probe(ctx, d, opts, acquire)
}
