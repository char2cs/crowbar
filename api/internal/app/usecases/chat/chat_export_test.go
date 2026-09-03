package chat

import (
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/conversation"
)

// ComposeContext exposes the {context} composition rule to this package's (external)
// tests. It is not production surface: this file is compiled only under `go test`.
//
// The rule is reached directly rather than through a spawn because one of its inputs
// comes from the process-wide config singleton, and config.Get MEMOISES on first use
// — so a spawn-level test could not present a blanked capabilities_instruction
// without depending on which test in the binary ran first. A composition rule tested
// through a memoised global is a test that passes for the wrong reason.
var ComposeContext = conversation.ComposeContext
