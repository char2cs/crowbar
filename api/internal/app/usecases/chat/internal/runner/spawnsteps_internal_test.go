package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
