//go:build unix

package descriptorcheck

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

const (
	pollEvery     = 250 * time.Millisecond
	eventTurnStop = "turn_stop"
)

// boot starts the TUI the way a chat does and keeps it up for the hook,
// turn and session-locate steps that need a live process.
func (l *live) boot(ctx context.Context) []Step {
	start := time.Now()
	tmp, err := l.tmpDir("boot")
	if err != nil {
		return []Step{fail("tui_boot", err.Error())}
	}
	t, err := l.spawn(ctx, tmp, nil)
	if err != nil {
		return []Step{fail("tui_boot", err.Error())}
	}
	defer t.close()
	s, parked := l.watchBoot(ctx, t)
	s.Elapsed = time.Since(start)
	switch {
	case s.Status == StatusFail:
		return []Step{s}
	case parked:
		why := "the CLI is parked on a prompt only a person answers; rerun with --cwd set to a directory it trusts"
		return []Step{s, skip("hooks", why), skip("turn", why), skip("session_locate", why)}
	}
	turn := l.turn(ctx, t)
	return []Step{s, l.hookStep(), turn, l.sessionLocate()}
}

// watchBoot holds the TUI for the boot window: an exit fails it, and a
// terminal prompt parks it (a user answers that in the terminal; the
// harness never does, since a default answer may be "exit").
func (l *live) watchBoot(ctx context.Context, t *term) (Step, bool) {
	deadline := time.Now().Add(l.opts.BootWindow)
	for time.Now().Before(deadline) {
		if ended, code := t.exited(pollEvery); ended {
			return fail("tui_boot", fmt.Sprintf("exited %d during boot: %s", code, tail(t.text(), 240))), false
		}
		if ctx.Err() != nil {
			return fail("tui_boot", ctx.Err().Error()), false
		}
		if prompt, ok := l.agent.MatchTerminalPrompt(t.text()); ok {
			return warn("tui_boot", fmt.Sprintf("up, but parked on a %s prompt", promptName(prompt.Kind))), true
		}
	}
	return pass("tui_boot", fmt.Sprintf("up for %s with no blocking prompt", l.opts.BootWindow)), false
}

func promptName(kind string) string {
	if kind == "" {
		return "terminal"
	}
	return fmt.Sprintf("%q", kind)
}

// turn types one prompt and waits for the turn to end, when asked to.
func (l *live) turn(ctx context.Context, t *term) Step {
	if !l.opts.Turn {
		return skip("turn", "not requested (--turn runs one real model turn)")
	}
	start := time.Now()
	if err := t.send(l.opts.Prompt); err != nil {
		return fail("turn", err.Error())
	}
	// A TUI reads a burst as a paste; the Enter must arrive after it.
	if ended, _ := t.exited(time.Second); ended {
		return fail("turn", "the CLI exited while the prompt was typed")
	}
	if err := t.send("\r"); err != nil {
		return fail("turn", err.Error())
	}
	deadline := start.Add(l.opts.TurnTimeout)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		if l.hooks.has(eventTurnStop) {
			s := pass("turn", "the turn ended and reported turn_stop")
			s.Elapsed = time.Since(start)
			return s
		}
		if ended, code := t.exited(pollEvery); ended {
			return fail("turn", fmt.Sprintf("exited %d mid-turn: %s", code, tail(t.text(), 240)))
		}
	}
	return fail("turn", fmt.Sprintf("no turn_stop within %s: %s", l.opts.TurnTimeout, tail(t.text(), 240)))
}

// hookStep parses every recorded hook with the descriptor, as the daemon would.
func (l *live) hookStep() Step {
	records := l.hooks.records()
	if len(records) == 0 {
		if l.opts.Turn {
			return fail("hooks", "no hook ever reached the relay")
		}
		return skip("hooks", "no hook fired at boot; this provider reports on the first turn (--turn)")
	}
	var names []string
	for _, rec := range records {
		if err := l.parseHook(rec); err != nil {
			return fail("hooks", fmt.Sprintf("%s: %v", rec.event, err))
		}
		names = append(names, rec.event)
	}
	return pass("hooks", "delivered and parsed: "+strings.Join(dedupe(names), ", "))
}

func (l *live) parseHook(rec hookRecord) error {
	if rec.event == "telemetry" {
		_, err := l.agent.ParseTelemetry(rec.payload, time.Now())
		return err
	}
	if _, declared := l.d.Events[rec.event]; !declared {
		return nil // wired for a relay-side purpose (idle), not an event
	}
	_, err := l.agent.ParseHook(rec.event, rec.payload, agents.ChannelHooks)
	return err
}

// sessionLocate checks that a session the CLI reported is where
// session.locate says, which is what the resume ladder's first rung reads.
func (l *live) sessionLocate() Step {
	if !l.opts.Turn {
		return skip("session_locate", "a session is written by its first turn (--turn)")
	}
	id := l.reportedSession()
	if id == "" {
		return fail("session_locate", "no hook reported a session id")
	}
	exists, declared := l.agent.SessionExists(id)
	switch {
	case !declared:
		return skip("session_locate", "the descriptor declares no session.locate")
	case exists:
		return pass("session_locate", "found session "+id)
	default:
		return fail("session_locate", fmt.Sprintf("session %s is not where session.locate points", id))
	}
}

func (l *live) reportedSession() string {
	for _, rec := range l.hooks.records() {
		if _, declared := l.d.Events[rec.event]; !declared {
			continue
		}
		if ev, err := l.agent.ParseHook(rec.event, rec.payload, agents.ChannelHooks); err == nil && ev.SessionID != "" {
			return ev.SessionID
		}
	}
	return ""
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
