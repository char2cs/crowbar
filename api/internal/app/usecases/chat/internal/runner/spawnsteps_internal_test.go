package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func positionalStep(value string) engineagents.InjectStep {
	return engineagents.InjectStep{Verb: "pass_arg", Args: map[string]any{"positional": value}}
}

func flagStep(arg, value string) engineagents.InjectStep {
	return engineagents.InjectStep{Verb: "pass_arg", Args: map[string]any{"arg": arg, "value": value}}
}

// TestRegression_MergeLeadingPositional_FoldsContextIntoTheMessage is the fix
// for a live bug: claude's switch-then-send resumes with the "while you were
// away" gap riding its own bare positional (resume_context_inject) and the
// user's real message riding a SEPARATE positional behind its own "--" guard
// (prompt_submit.resume) — two distinct trailing positionals. Confirmed live
// that claude's CLI attends to only the first and silently drops the second:
// the gap got acknowledged, the user's actual question never reached the
// CLI's own user_prompt hook, and the client saw nothing but a bare
// acknowledgement forever. Folding both into ONE positional argv token,
// still behind exactly one "--" guard, is what a single-positional CLI can
// actually receive as one turn.
func TestRegression_MergeLeadingPositional_FoldsContextIntoTheMessage(t *testing.T) {
	context := []engineagents.InjectStep{positionalStep("<system-reminder>gap</system-reminder>")}
	final := []engineagents.InjectStep{positionalStep("--"), positionalStep("what did I miss?")}

	merged, ok := mergeLeadingPositional(context, final)

	require := assert.New(t)
	require.True(ok)
	require.Len(merged, 2)
	require.Equal("--", merged[0].Args["positional"])
	require.Equal(
		"<system-reminder>gap</system-reminder>\n\nwhat did I miss?",
		merged[1].Args["positional"],
	)
}

// TestRegression_MergeLeadingPositional_NoMessage_LeavesContextStandalone
// covers SwitchProvider's own silent resume spawn (an empty promptMessage,
// so finalSteps is nil): nothing to fold into, so the context step must be
// emitted exactly as before — this is the shape
// TestSwitchProvider_ClaudeSwitchBack_ResumesAndPointsAtTheGap already pins
// end-to-end, and a merge here would silently swallow the whole handoff.
func TestRegression_MergeLeadingPositional_NoMessage_LeavesContextStandalone(t *testing.T) {
	context := []engineagents.InjectStep{positionalStep("<system-reminder>gap</system-reminder>")}

	_, ok := mergeLeadingPositional(context, nil)

	assert.False(t, ok, "no final steps to fold into — the caller must append context standalone")
}

// TestRegression_MergeLeadingPositional_FlagFinalStep_NeverMerges guards a
// provider whose own prompt delivery is flag-based (not positional): merging
// context into a flag's VALUE would silently corrupt an unrelated argument
// instead of delivering the message.
func TestRegression_MergeLeadingPositional_FlagFinalStep_NeverMerges(t *testing.T) {
	context := []engineagents.InjectStep{positionalStep("<system-reminder>gap</system-reminder>")}
	final := []engineagents.InjectStep{flagStep("--message", "what did I miss?")}

	_, ok := mergeLeadingPositional(context, final)

	assert.False(t, ok, "a flag-shaped final step must never receive a merged positional value")
}

// TestRegression_MergeLeadingPositional_MultiStepContext_NeverMerges guards
// against silently dropping earlier context steps: this helper only knows
// how to fold ONE bare positional, so a provider whose ContextSteps renders
// more than one step must fall back to the caller's normal append path
// rather than losing everything but the last.
func TestRegression_MergeLeadingPositional_MultiStepContext_NeverMerges(t *testing.T) {
	context := []engineagents.InjectStep{
		positionalStep("part one"),
		positionalStep("part two"),
	}
	final := []engineagents.InjectStep{positionalStep("--"), positionalStep("what did I miss?")}

	_, ok := mergeLeadingPositional(context, final)

	assert.False(t, ok)
}

// apiResumeTestDescriptor is codex's own shape reduced to what this decision
// touches: api transport for prompt, NO hotswap (so apiOwnsResume is true), a
// native resume arg, and a resume-time context inject. It is a real descriptor
// loaded through the real registry rather than a hand-faked Agent, so
// TransportFor/Capabilities/ResumeArg/ContextSteps all answer the way
// production's do.
const apiResumeTestDescriptor = `
id: apiresume-test
spawn:
  cmd: acme
  interactive_required: true
session:
  resume: { arg: "resume {id}" }
resume_context_inject:
  - pass_arg: { positional: "<system-reminder>{context}</system-reminder>" }
events:
  session_start:
    in: thread/started
    map: { session_id: thread.id }
  turn_stop:
    in: turn/completed
    map:
      session_id: threadId
      message: "turn.items[type=agentMessage].text"
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve: [acme, serve]
    handshake: { call: initialize }
`

func loadAPIResumeDescriptor(t *testing.T) engineagents.Agent {
	t.Helper()
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(home, "descriptors", "apiresume-test.yaml"), []byte(apiResumeTestDescriptor), 0o600))
	d, err := engineagents.New().Get(context.Background(), home, "apiresume-test")
	require.NoError(t, err)
	return d
}

// TestBuildSpawnSteps_ApiResumes_WithholdsTheNativeResumeAndTheGap is the
// WITHHOLDING half of the contract, pinned on the decision that now carries it.
// A LIVE api connection has already run thread/resume on this session, and codex
// permits exactly one writer per thread (a writer-lock file, confirmed on disk):
// handing the companion PTY the same id as a native `resume {id}` kills it
// outright (exit 1, the switch silently reverts), and handing it the gap
// document instead makes it answer as a second, disconnected conversation the
// api connection knows nothing about. Both confirmed live. So both are withheld.
func TestBuildSpawnSteps_ApiResumes_WithholdsTheNativeResumeAndTheGap(t *testing.T) {
	d := loadAPIResumeDescriptor(t)
	require.True(t, apiOwnsResume(d), "this descriptor must declare that api transport owns resuming")
	resume := resumeInjectionSteps(d, "sess-1")
	require.NotEmpty(t, resume, "the descriptor's own native resume argv must render")

	steps := buildSpawnSteps(d, true, true, true, engineagents.Selection{}, resume, nil)

	assert.Empty(t, steps,
		"a live api connection owns this resume: neither the native id nor the gap may reach the PTY: %v", steps)
}

// TestBuildSpawnSteps_NoLiveAPIConnection_CarriesTheNativeResume is the bug this
// pair exists for. The same descriptor, but nothing actually resumed the session:
// `serve` never forked, its socket never appeared, the handshake was refused, or
// the transport is off for this process (design spec §2.2b — the session then
// runs over hooks alone). There is no first writer to collide with, the PTY IS
// the conversation, and withholding the resume made it mint a brand new thread —
// the chat's own history silently abandoned on every restart and switch back.
func TestBuildSpawnSteps_NoLiveAPIConnection_CarriesTheNativeResume(t *testing.T) {
	d := loadAPIResumeDescriptor(t)
	resume := resumeInjectionSteps(d, "sess-1")

	steps := buildSpawnSteps(d, true, true, false, engineagents.Selection{}, resume, nil)

	require.GreaterOrEqual(t, len(steps), len(resume))
	assert.Equal(t, resume, steps[:len(resume)],
		"the native resume must lead the argv, ahead of selection and context: %v", steps)
	assert.Greater(t, len(steps), len(resume),
		"the gap document must ride the argv too, since no api connection can carry it: %v", steps)
}

// TestApiResumes_RequiresBOTHTheDeclarationAndALiveConnection pins the two
// halves against each other on Runners itself: the descriptor answer alone is
// what the buggy version asked, and it is true here for both cases.
func TestApiResumes_RequiresBOTHTheDeclarationAndALiveConnection(t *testing.T) {
	d := loadAPIResumeDescriptor(t)
	rs := &Runners{apiConns: newAPIConnRegistry()}

	assert.False(t, rs.apiResumes(d, "runner-1"),
		"no connection registered: the PTY must be left to resume the session itself")

	rs.apiConns.set("runner-1", &apiconn{})

	assert.True(t, rs.apiResumes(d, "runner-1"),
		"a live connection holds the thread: the PTY must not become a second writer on it")
	assert.False(t, rs.apiResumes(d, "runner-2"),
		"scoped to the runner asked about, never to any connection anywhere")
}
