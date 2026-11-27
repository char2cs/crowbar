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

func TestRegression_MergeLeadingPositional_NoMessage_LeavesContextStandalone(t *testing.T) {
	context := []engineagents.InjectStep{positionalStep("<system-reminder>gap</system-reminder>")}

	_, ok := mergeLeadingPositional(context, nil)

	assert.False(t, ok, "no final steps to fold into — the caller must append context standalone")
}

func TestRegression_MergeLeadingPositional_FlagFinalStep_NeverMerges(t *testing.T) {
	context := []engineagents.InjectStep{positionalStep("<system-reminder>gap</system-reminder>")}
	final := []engineagents.InjectStep{flagStep("--message", "what did I miss?")}

	_, ok := mergeLeadingPositional(context, final)

	assert.False(t, ok, "a flag-shaped final step must never receive a merged positional value")
}

func TestRegression_MergeLeadingPositional_MultiStepContext_NeverMerges(t *testing.T) {
	context := []engineagents.InjectStep{
		positionalStep("part one"),
		positionalStep("part two"),
	}
	final := []engineagents.InjectStep{positionalStep("--"), positionalStep("what did I miss?")}

	_, ok := mergeLeadingPositional(context, final)

	assert.False(t, ok)
}

const apiResumeTestDescriptor = `
id: apiresume-test
spawn:
  cmd: acme
  interactive_required: true
session: { resume: { arg: "resume {id}" }, persists: turn_start }
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
    delivery: stdio
    format: ndjson
    serve: [acme, serve]
    handshake: { call: initialize, notify: initialized }
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

func TestBuildSpawnSteps_ThePTYCarriesTheNativeResumeAndTheGap(t *testing.T) {
	d := loadAPIResumeDescriptor(t)
	resume := resumeInjectionSteps(d, "sess-1")

	steps := buildSpawnSteps(d, true, true, engineagents.Selection{}, resume, nil)

	require.GreaterOrEqual(t, len(steps), len(resume))
	assert.Equal(t, resume, steps[:len(resume)],
		"the native resume must lead the argv, ahead of selection and context: %v", steps)
	assert.Greater(t, len(steps), len(resume),
		"the gap document must ride the argv too, since no api connection can carry it: %v", steps)
}
