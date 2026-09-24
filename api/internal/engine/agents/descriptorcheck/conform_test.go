//go:build unix

package descriptorcheck_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

type run struct {
	descriptor string
	transcript string
	turn       bool
	edit       func(string) string
}

// conform runs a fake-CLI descriptor through the live harness, the fake
// following the named transcript.
func conform(t *testing.T, r run) descriptorcheck.LiveReport {
	t.Helper()
	bin, err := os.Executable()
	require.NoError(t, err)
	raw, err := os.ReadFile(filepath.Join("testdata", "fakecli", r.descriptor))
	require.NoError(t, err)
	doc := strings.ReplaceAll(string(raw), "BIN", bin)
	if r.edit != nil {
		doc = r.edit(doc)
	}
	script, err := filepath.Abs(filepath.Join("testdata", "fakecli", "transcripts", r.transcript+".yaml"))
	require.NoError(t, err)
	// The daemon and its CLIs share one environment; so do the harness and the fake.
	t.Setenv("FAKECLI_HOME", t.TempDir())
	env := append(os.Environ(), "FAKECLI_SCRIPT="+script)
	return descriptorcheck.Conform(context.Background(), []byte(doc), descriptorcheck.LiveOptions{
		Turn:         r.turn,
		Prompt:       "hello fake",
		BootWindow:   1500 * time.Millisecond,
		ResumeWindow: 3 * time.Second,
		TurnTimeout:  10 * time.Second,
		ServeWindow:  3 * time.Second,
		Env:          env,
	})
}

func statuses(rep descriptorcheck.LiveReport) map[string]descriptorcheck.Status {
	out := map[string]descriptorcheck.Status{}
	for _, s := range rep.Steps {
		out[s.Name] = s.Status
	}
	return out
}

func step(t *testing.T, rep descriptorcheck.LiveReport, name string) descriptorcheck.Step {
	t.Helper()
	for _, s := range rep.Steps {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("no %s step in %+v", name, rep.Steps)
	return descriptorcheck.Step{}
}

func TestConform_AHealthyCLIPassesEveryStep(t *testing.T) {
	rep := conform(t, run{descriptor: "descriptor.yaml", transcript: "healthy", turn: true})

	assert.Equal(t, map[string]descriptorcheck.Status{
		"binary": "pass", "version": "pass", "flags": "pass", "tui_boot": "pass",
		"hooks": "pass", "turn": "pass", "session_locate": "pass", "resume_unknown": "pass",
		"app_server": "skip",
	}, statuses(rep), "%+v", rep.Steps)
	assert.True(t, rep.OK())
	assert.Contains(t, step(t, rep, "hooks").Detail, "session_start, user_prompt, turn_stop")
	assert.Contains(t, step(t, rep, "version").Detail, "1.4.2")
}

func TestConform_ACLIThatDiesAtBootFails(t *testing.T) {
	rep := conform(t, run{descriptor: "descriptor.yaml", transcript: "crash_at_boot", turn: true})

	boot := step(t, rep, "tui_boot")
	assert.Equal(t, descriptorcheck.StatusFail, boot.Status)
	assert.Contains(t, boot.Detail, "exited 3")
	assert.Contains(t, boot.Detail, "config is corrupt", "the CLI's own last words are shown")
	assert.False(t, rep.OK())
}

// A prompt only a person answers parks the run; the harness never answers it
// (the default choice may be "exit").
func TestConform_ATrustPromptParksTheRunWithoutAnsweringIt(t *testing.T) {
	rep := conform(t, run{descriptor: "descriptor.yaml", transcript: "trust_prompt", turn: true})

	got := statuses(rep)
	assert.Equal(t, descriptorcheck.StatusWarn, got["tui_boot"])
	assert.Equal(t, descriptorcheck.StatusSkip, got["hooks"])
	assert.Equal(t, descriptorcheck.StatusSkip, got["turn"])
	assert.Equal(t, descriptorcheck.StatusWarn, got["resume_unknown"])
	assert.Contains(t, step(t, rep, "tui_boot").Detail, "workspace_trust")
	assert.True(t, rep.OK(), "a parked prompt is reported, not failed")
}

// The resume probe reads an early exit as "that session is gone"; a CLI that
// stays up on an unknown session would pass a lost session off as live.
func TestConform_AResumeOfAnUnknownSessionMustEnd(t *testing.T) {
	rep := conform(t, run{descriptor: "descriptor.yaml", transcript: "resume_hangs"})

	resume := step(t, rep, "resume_unknown")
	assert.Equal(t, descriptorcheck.StatusFail, resume.Status)
	assert.Contains(t, resume.Detail, "still running")
}

func TestConform_AHookPayloadTheDescriptorCannotReadFails(t *testing.T) {
	rep := conform(t, run{descriptor: "descriptor.yaml", transcript: "foreign_payload"})

	hooks := step(t, rep, "hooks")
	assert.Equal(t, descriptorcheck.StatusFail, hooks.Status)
	assert.Contains(t, hooks.Detail, "session_start")
}

func TestConform_ASessionWrittenElsewhereFailsTheLocateStep(t *testing.T) {
	rep := conform(t, run{descriptor: "descriptor.yaml", transcript: "session_elsewhere", turn: true})

	assert.Equal(t, descriptorcheck.StatusPass, step(t, rep, "turn").Status)
	locate := step(t, rep, "session_locate")
	assert.Equal(t, descriptorcheck.StatusFail, locate.Status)
	assert.Contains(t, locate.Detail, "not where session.locate points")
}

// Without --turn, a provider that reports hooks only once a turn starts
// (codex) is skipped, not failed; an undocumented flag only warns.
func TestConform_WithoutATurnAQuietBootIsSkippedNotFailed(t *testing.T) {
	rep := conform(t, run{descriptor: "descriptor.yaml", transcript: "quiet_boot"})

	got := statuses(rep)
	assert.Equal(t, descriptorcheck.StatusSkip, got["hooks"])
	assert.Equal(t, descriptorcheck.StatusSkip, got["turn"])
	assert.Equal(t, descriptorcheck.StatusSkip, got["session_locate"])
	assert.Equal(t, descriptorcheck.StatusWarn, got["flags"])
	assert.Contains(t, step(t, rep, "flags").Detail, "--fake-tui")
	assert.True(t, rep.OK())
}

func TestConform_AMissingBinaryStopsAtOnce(t *testing.T) {
	rep := conform(t, run{descriptor: "descriptor.yaml", transcript: "healthy", edit: func(s string) string {
		return strings.Replace(s, "  cmd: /", "  cmd: /nonexistent/", 1)
	}})

	require.Len(t, rep.Steps, 1)
	assert.Equal(t, descriptorcheck.StatusFail, rep.Steps[0].Status)
	assert.Equal(t, "binary", rep.Steps[0].Name)
}

func TestConform_AVersionOutsideTheDeclaredRangeFails(t *testing.T) {
	rep := conform(t, run{descriptor: "descriptor.yaml", transcript: "healthy", edit: func(s string) string {
		return s + "protocol_version: { min: \"2.0.0\" }\n"
	}})

	assert.Equal(t, descriptorcheck.StatusFail, step(t, rep, "version").Status)
}

func TestConform_AStaticErrorRunsNothingLive(t *testing.T) {
	rep := conform(t, run{descriptor: "descriptor.yaml", transcript: "healthy", edit: func(s string) string {
		return strings.Replace(s, "hooks_injection:", "hooks_injecton:", 1)
	}})

	require.Len(t, rep.Steps, 1)
	assert.Equal(t, "static", rep.Steps[0].Name)
	assert.False(t, rep.OK())
}

func TestConform_TheAppServerMustAnswerInitialize(t *testing.T) {
	cases := map[string]descriptorcheck.Status{
		"serve_answers": descriptorcheck.StatusPass,
		"serve_hangs":   descriptorcheck.StatusFail,
		"serve_exits":   descriptorcheck.StatusFail,
	}
	for transcript, want := range cases {
		t.Run(transcript, func(t *testing.T) {
			rep := conform(t, run{descriptor: "serve.yaml", transcript: transcript})
			s := step(t, rep, "app_server")
			assert.Equal(t, want, s.Status, s.Detail)
		})
	}
}
