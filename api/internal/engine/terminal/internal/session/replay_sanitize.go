package session

import "regexp"

// replaySanitizers strip escape sequences that must NOT be re-processed when a
// session's scrollback snapshot is replayed to a (re)attaching client.
//
// Replaying raw scrollback makes xterm re-act to control sequences that were
// meaningful when first emitted but are wrong on replay:
//
//   - Terminal QUERIES (Device Attributes "ESC [ ... c", Device Status Report
//     "ESC [ ... n", OSC color queries "ESC ] N ; ? BEL") — xterm answers them,
//     and the answers are echoed at the restored shell's prompt as the visible
//     "^[[?1;2c" / "]11;rgb:..." garbage.
//   - App-only private modes (mouse tracking, focus reporting, bracketed paste,
//     alternate screen) — re-enabling them on a restored *shell* (no app to
//     consume the events) leaks mouse/focus escapes into the prompt.
//   - OSC set-title — re-applies a stale/garbled title to the tab.
//
// None of these affect the VISUAL restoration of the screen: SGR colors, cursor
// moves, and erases are deliberately left untouched, so the restored scrollback
// still renders identically. Stripping only removes the on-attach garbage.
var replaySanitizers = []*regexp.Regexp{
	// Device Attributes request AND response: ESC [ <params> c
	regexp.MustCompile(`\x1b\[[0-9;?>=]*c`),
	// Device Status Report request: ESC [ <params> n
	regexp.MustCompile(`\x1b\[[0-9;]*n`),
	// OSC queries (e.g. color "ESC ] 11 ; ?"), terminated by BEL or ST.
	regexp.MustCompile(`\x1b\][0-9;]*\?(?:\x07|\x1b\\)`),
	// OSC set-title (0/1/2), terminated by BEL or ST. The title body may itself
	// contain embedded ESC sequences — some zsh prompt-title hooks put the whole
	// ANSI-COLORED prompt inside the OSC parameter (e.g. ESC]2;worktree
	// [ESC[01;32m0ESC[00m] %BEL). A plain [^ESC]* body bails at that first inner
	// ESC and fails to strip such a title, so allow ESC-that-is-not-ST inside the
	// body (ESC followed by any byte except '\', since ESC '\' is the ST
	// terminator).
	regexp.MustCompile(`\x1b\][012];(?:[^\x07\x1b]|\x1b[^\\])*(?:\x07|\x1b\\)`),
	// App-only private modes (set or reset): mouse 1000-1006/1015, focus 1004,
	// bracketed paste 2004, alternate screen 47/1047/1049.
	regexp.MustCompile(`\x1b\[\?(?:1000|1001|1002|1003|1004|1005|1006|1015|2004|1049|1047|47)[hl]`),
}

// sanitizeReplaySnapshot returns snap with replay-unsafe escape sequences removed.
// It only allocates when something is actually stripped.
func sanitizeReplaySnapshot(snap []byte) []byte {
	out := snap
	for _, re := range replaySanitizers {
		if re.Match(out) {
			out = re.ReplaceAll(out, nil)
		}
	}
	return out
}
