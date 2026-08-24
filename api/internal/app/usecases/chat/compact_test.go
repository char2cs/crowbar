package chat_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
)

// compactingDescriptorBody declares the compaction gesture the way claude does: no API
// for it, so the trigger is the slash command injected over the prompt transport.
const compactingDescriptorBody = `
id: claude
display_name: Compacting
spawn:
  cmd: claude
  interactive_required: true
session:
  resume: { arg: "--resume {id}" }
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh:
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "{message}" }
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last_assistant_message
  compact_start:
    out: prompt
    send:
      text: "/compact"
runtime:
  transport: hooks
  hooks:
    format: json
`

// The same provider WITHOUT the gesture: everything else identical, so a difference in
// behaviour can only come from compact_start's absence.
const nonCompactingDescriptorBody = `
id: claude
display_name: NotCompacting
spawn:
  cmd: claude
  interactive_required: true
session:
  resume: { arg: "--resume {id}" }
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh:
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "{message}" }
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
`

// Compaction reaches the CLI as the provider's OWN gesture — Crowbar cannot compact
// anything itself, the context belongs to the provider.
func TestCompact_SendsTheProvidersDeclaredGesture(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", compactingDescriptorBody)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "native-session")

	require.NoError(t, f.usecase.Compact(f.ctx, chatID))

	call := f.term.calls[f.term.callCount()-1]
	joined := strings.Join(call.argv, " ")
	assert.Contains(t, joined, "/compact",
		"the provider's declared gesture must reach the CLI verbatim")
}

// Key-presence IS the capability. A provider that declares no gesture cannot be asked,
// and must say so rather than silently doing nothing — a compact button that appears
// to work and does not is worse than one that is absent.
func TestCompact_AProviderWithNoGestureIsNotFound(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", nonCompactingDescriptorBody)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "native-session")

	before := f.term.callCount()
	err := f.usecase.Compact(f.ctx, chatID)

	require.Error(t, err)
	assert.ErrorIs(t, err, apperr.ErrNotFound)
	assert.Equal(t, before, f.term.callCount(),
		"a provider that cannot compact must have nothing sent to it")
}

// A chat that does not exist must fail before anything is sent anywhere.
func TestCompact_AnUnknownChatSendsNothing(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", compactingDescriptorBody)

	before := f.term.callCount()
	err := f.usecase.Compact(f.ctx, "no-such-chat")

	require.Error(t, err)
	assert.Equal(t, before, f.term.callCount(),
		"an unknown chat must reach no CLI at all")
}
