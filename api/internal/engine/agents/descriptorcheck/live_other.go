//go:build !unix

package descriptorcheck

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func runLive(context.Context, []byte, *spec.Descriptor, LiveOptions) []Step {
	return []Step{{Name: "setup", Status: StatusFail, Detail: "live conformance drives the CLI in a unix PTY"}}
}
