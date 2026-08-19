// Package rewake answers the one question the rewake delivery channel adds to
// the user-prompt hook: is this prompt the one CROWBAR just handed the provider,
// and if so, what did the user actually type?
//
// It exists because rewake gives up the property that made every other delivery
// legible. Under a restart, the process that emits a prompt hook is the process
// Crowbar spawned with that prompt in its argv, so "who sent this" is answered by
// which runner it came from. Under rewake, ONE live runner emits both the user's
// messages and the provider's own harness injections, on the same event, wrapped
// in the same markup — and the measured wrapper OPENS with the exact string the
// harness's background-subagent reports open with. Nothing structural separates
// them. The sentinel is what does, and it is Crowbar's own bytes, registered with
// the provider at spawn and never written by anything else.
//
// The failure this package must never produce is a MISLABELLED prompt in either
// direction: the user quoted saying what a subagent reported, or the user's own
// message filed as the harness's and dropped from their side of the
// conversation. So the match is exact and anchored, and anything it is not
// certain about it declines — which lands on the classification that ran before
// this channel existed.
package rewake

import (
	"regexp"
	"strings"
	"sync"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// compiled caches one descriptor pattern per pattern string. Descriptors are
// re-read per resolve, so without this every user-prompt hook would recompile the
// same regex.
var compiled sync.Map // pattern string -> *regexp.Regexp

// Declared reports whether this provider delivers prompts by rewake at all. It is
// the capability read: a provider that does not never has its prompts inspected,
// and behaves exactly as it did before this package existed.
func Declared(d *spec.Descriptor) bool {
	if d == nil || d.Presentation.PromptSubmit == nil {
		return false
	}
	ps := d.Presentation.PromptSubmit
	return ps.Strategy == spec.DeliveryRewakeHook && ps.Rewake != nil
}

// Sentinel is the prefix the provider must be told to put in front of a collected
// prompt, and Summary the one-line description it shows in its own terminal.
// Both are read from the descriptor so the value registered at spawn and the
// value matched at ingest can never drift apart.
func Sentinel(d *spec.Descriptor) string {
	if !Declared(d) {
		return ""
	}
	return d.Presentation.PromptSubmit.Rewake.Sentinel
}

func Summary(d *spec.Descriptor) string {
	if !Declared(d) {
		return ""
	}
	return d.Presentation.PromptSubmit.Rewake.Summary
}

// WakeStatus is the exit status this provider's collector must leave with for the
// collected message to be taken. Zero for a provider that declares no channel,
// and a collector handed zero delivers nothing rather than printing into a
// session that would ignore it.
func WakeStatus(d *spec.Descriptor) int {
	if !Declared(d) {
		return 0
	}
	return d.Presentation.PromptSubmit.Rewake.WakeStatus
}

// Match recovers the user's own text from a prompt this provider delivered by
// rewake, and reports false for everything else.
//
// False is the SAFE answer and the common one: a prompt typed into the provider's
// own composer, a harness injection, a provider that declares no rewake at all —
// every one of them lands here, and every one of them is then classified exactly
// as it was before rewake existed.
//
// True is only returned for a prompt that matches the descriptor's whole anchored
// wrapper INCLUDING the sentinel. The sentinel is what makes that a proof rather
// than a resemblance: it is a Crowbar-authored string the provider was handed at
// spawn, so a document merely SHAPED like the wrapper — a harness notification,
// or a person pasting one into the composer — cannot carry it by accident.
func Match(d *spec.Descriptor, prompt string) (string, bool) {
	if !Declared(d) || prompt == "" {
		return "", false
	}
	rw := d.Presentation.PromptSubmit.Rewake
	// The sentinel is checked before the regex for one reason that is not
	// performance: it states the load-bearing condition in one line, where a
	// reader can see that no path returns true without it.
	if rw.Sentinel == "" || !strings.Contains(prompt, rw.Sentinel) {
		return "", false
	}
	re := pattern(rw.StripPattern())
	if re == nil {
		return "", false
	}
	groups := re.FindStringSubmatch(prompt)
	if groups == nil {
		return "", false
	}
	for i, name := range re.SubexpNames() {
		if name == "message" {
			return groups[i], true
		}
	}
	return "", false
}

func pattern(p string) *regexp.Regexp {
	if p == "" {
		return nil
	}
	if cached, ok := compiled.Load(p); ok {
		re, _ := cached.(*regexp.Regexp)
		return re
	}
	// A pattern that does not compile yields NO match rather than a panic. The
	// descriptor rule refuses one at load, so this is only reachable through a
	// hand-edited on-disk override — and there the honest degradation is "this
	// provider has no rewake channel", not a dead daemon.
	re, err := regexp.Compile(p)
	if err != nil {
		re = nil
	}
	compiled.Store(p, re)
	return re
}
