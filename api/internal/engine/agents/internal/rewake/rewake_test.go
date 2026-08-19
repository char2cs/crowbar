package rewake_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/rewake"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// shipped is the descriptor claude.yaml ACTUALLY ships, not a fixture shaped like
// it. Every case below therefore tests the sentence and the pattern a user's
// machine will run, so a descriptor edit that broke either fails here.
func shipped(t *testing.T) *spec.Descriptor {
	t.Helper()
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "claude")
	require.NoError(t, err, "resolve the shipped claude descriptor")
	return d
}

// wrap builds the payload claude 2.1.235 was MEASURED to produce, byte for byte,
// from a captured UserPromptSubmit stdin on a live interactive PTY (2026-08-18).
// The literal shape is reproduced here rather than derived from the descriptor,
// so a pattern edited to match itself still has to match what claude sends.
func wrap(summary, sentinel, text string) string {
	return "<task-notification>\n" +
		"<summary>" + summary + "</summary>\n" +
		"</task-notification>\n" +
		"<system-reminder>\n" +
		sentinel + " " + text + "\n" +
		"</system-reminder>"
}

// measuredRewake is the VERBATIM capture from that session, elisions and all —
// the probe registered its own sentinel and summary, which is why they are not
// the shipped ones. It is kept because it is the only string in this file nobody
// composed: it is what came out of the CLI.
const measuredRewake = "<task-notification>\n" +
	"<summary>Crowbar chat prompt</summary>\n" +
	"</task-notification>\n" +
	"<system-reminder>\n" +
	"CROWBAR-REWAKE-SENTINEL:: Reply with exactly the word ACK2 and nothing else.\n" +
	"</system-reminder>"

// measuredTaskNotification is a REAL background-subagent report from claude
// 2.1.234, captured the same way. It is the trap: it opens with the identical
// `<task-notification>` tag a rewake does, and it must never be unwrapped as
// something the user typed.
const measuredTaskNotification = `<task-notification>
<task-id>aa3b60603214670cc</task-id>
<tool-use-id>toolu_01CZ…</tool-use-id>
<output-file>…</output-file>
<status>completed</status>
<summary>Agent "Reply with PONG" finished</summary>
<note>A task-notification fires each time this agent stops with no live background children of its own. …</note>
<result>PONG</result>
<usage><subagent_tokens>18471</subagent_tokens><tool_uses>0</tool_uses><duration_ms>1337</duration_ms></usage>
</task-notification>`

func TestMatch_RecoversTheUsersTextFromTheShippedWrapper(t *testing.T) {
	d := shipped(t)
	text := "why does the sidebar flicker on a workspace switch?"

	got, ok := rewake.Match(d, wrap(rewake.Summary(d), rewake.Sentinel(d), text))

	require.True(t, ok, "the shipped strip pattern must match the measured wrapper")
	assert.Equal(t, text, got)
}

// TestMatch_TheMeasuredCaptureShape is the pattern held against the exact bytes
// claude sent, with the probe's own sentinel substituted into a descriptor. It
// exists so a pattern that only ever meets wrap() above cannot drift away from
// the capture.
func TestMatch_TheMeasuredCaptureShape(t *testing.T) {
	d := shipped(t)
	d.Presentation.PromptSubmit.Rewake.Sentinel = "CROWBAR-REWAKE-SENTINEL::"

	got, ok := rewake.Match(d, measuredRewake)

	require.True(t, ok)
	assert.Equal(t, "Reply with exactly the word ACK2 and nothing else.", got)
}

// TestMatch_AHarnessNotificationIsNeverTheUsers is the whole point of the
// sentinel, in the direction that loses a person's words.
func TestMatch_AHarnessNotificationIsNeverTheUsers(t *testing.T) {
	_, ok := rewake.Match(shipped(t), measuredTaskNotification)

	assert.False(t, ok, "a real subagent report must not be unwrapped as a user prompt")
}

func TestMatch_DeclinesEverythingItCannotProve(t *testing.T) {
	d := shipped(t)
	sentinel, summary := rewake.Sentinel(d), rewake.Summary(d)

	testCases := map[string]string{
		"a prompt typed into the composer": "say only ACK",
		// The shape without the proof. This is the exact document a harness
		// injection could grow into, and it must still be refused.
		"the wrapper with no sentinel": "<task-notification>\n<summary>" + summary +
			"</summary>\n</task-notification>\n<system-reminder>\nsay only ACK\n</system-reminder>",
		// The proof with no shape: the sentence pasted into a message. Contains is
		// necessary for a match and nowhere near sufficient.
		"the sentinel quoted in prose": "what does " + sentinel + " mean?",
		// Anchoring at the FRONT: anything before the tag means this is not the
		// document, it is a message containing one.
		"the wrapper with a preamble": "look at this:\n" + wrap(summary, sentinel, "say only ACK"),
		// Anchoring at the BACK, which is what stops a harness report that happens
		// to embed a wrapper from being read as the user's last line.
		"the wrapper with a trailer": wrap(summary, sentinel, "say only ACK") + "\nand then this",
	}
	for name, prompt := range testCases {
		t.Run(name, func(t *testing.T) {
			_, ok := rewake.Match(d, prompt)
			assert.False(t, ok)
		})
	}
}

// TestMatch_KeepsAMultiLineMessageWhole pins the greedy capture: a message that
// itself contains the closing tag must come back complete, not truncated at the
// first one.
func TestMatch_KeepsAMultiLineMessageWhole(t *testing.T) {
	d := shipped(t)
	text := "here is the payload:\n</system-reminder>\nwhat does that tag do?"

	got, ok := rewake.Match(d, wrap(rewake.Summary(d), rewake.Sentinel(d), text))

	require.True(t, ok)
	assert.Equal(t, text, got)
}

// TestDeclared_ProviderWithoutTheChannel is the degradation story: codex declares
// restart_tui, so nothing here ever fires for it and its prompts are classified
// exactly as they were before this package existed.
func TestDeclared_ProviderWithoutTheChannel(t *testing.T) {
	codex, err := descriptor.Resolve(context.Background(), t.TempDir(), "codex")
	require.NoError(t, err)

	assert.False(t, rewake.Declared(codex))
	assert.Empty(t, rewake.Sentinel(codex))
	assert.Empty(t, rewake.Summary(codex))

	_, ok := rewake.Match(codex, measuredRewake)
	assert.False(t, ok)

	assert.False(t, rewake.Declared(nil))
	_, ok = rewake.Match(nil, measuredRewake)
	assert.False(t, ok)
}

// TestMatch_AnUncompilablePatternHasNoChannel covers the on-disk override the
// descriptor rule cannot reach: a broken pattern costs this provider its rewake
// channel and nothing else.
func TestMatch_AnUncompilablePatternHasNoChannel(t *testing.T) {
	d := shipped(t)
	d.Presentation.PromptSubmit.Rewake.Strip = "(?P<message>" // never closed

	_, ok := rewake.Match(d, wrap(rewake.Summary(d), rewake.Sentinel(d), "say only ACK"))

	assert.False(t, ok)
}

// TestSentinel_IsSafeInsideTheJSONItIsWrittenInto guards a break that would only
// show up as a CLI that refuses to start: the same string is interpolated into
// the JSON body of settings.json, where a quote or a backslash would end the
// string early.
func TestSentinel_IsSafeInsideTheJSONItIsWrittenInto(t *testing.T) {
	d := shipped(t)

	for name, value := range map[string]string{
		"sentinel": rewake.Sentinel(d),
		"summary":  rewake.Summary(d),
	} {
		t.Run(name, func(t *testing.T) {
			require.NotEmpty(t, value)
			assert.False(t, strings.ContainsAny(value, "\"\\\n\r"),
				"must not carry a quote, a backslash or a newline")
		})
	}
}
