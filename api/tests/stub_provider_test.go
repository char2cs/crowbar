//go:build integration

package tests

import (
	"testing"
)

// promoteStubProviderDescriptorYAML is cwdstub with one difference that
// matters here: it prints the last TWO segments of its real working directory
// rather than the whole path.
//
// readMarkedValue stops at the first control byte, and a full temp-dir path
// wraps past the PTY's column width — so two DIFFERENT worktrees under the
// same temp root read back as the same truncated prefix, and an assertion
// that the promoted CLI moved would pass whether it moved or not. Every
// managed worktree ends in a literal "worktree" leaf under a directory named
// for its BRANCH (worktreepath.WorkspaceRoot), so the branch segment is
// exactly the part that differs and the pair is short enough to survive the
// wrap.
const promoteStubProviderDescriptorYAML = `id: promotestub
spawn:
  cmd: "sh"
  args: ["-c", "echo CROWBAR_CWD:$(basename $(dirname $(pwd)))/$(basename $(pwd)); exec cat"]
  interactive_required: true
session: { resume: { arg: "--resume {id}" }, persists: turn_start }
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  user_prompt:
    in: user_prompt
    map:
      message: prompt
  turn_stop:
    in: turn_stop
    map:
      session_id: session_id
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
`

func writePromoteStubProviderDescriptor(t *testing.T, h *harness) {
	t.Helper()
	writeProviderDescriptor(t, h, "promotestub", promoteStubProviderDescriptorYAML)
}
