//go:build integration

package tests

import (
	"testing"
)

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
