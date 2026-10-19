package agents

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func (a *agent) SessionAssignArg() (string, bool) {
	if a.spec.Session.Assign == nil || a.spec.Session.Assign.Arg == "" {
		return "", false
	}
	return a.spec.Session.Assign.Arg, true
}

// SessionPersists is when the CLI has written a session it can resume.
func (a *agent) SessionPersists() string { return a.spec.Session.Persists }

// RefusedResume is refused only for the declared exit code AND one of the
// descriptor's needles on the screen, matched without whitespace so a line the
// terminal wrapped across rows still counts. Any other exit says nothing.
func (a *agent) RefusedResume(exitCode int, output string) (string, bool) {
	r := a.spec.Session.Refused
	if r == nil || exitCode != r.ExitCode {
		return "", false
	}
	screen := squeezeScreen(output)
	for _, needle := range r.Needles {
		if line, ok := screen.find(needle); ok {
			return line, true
		}
	}
	return "", false
}

// squeezedScreen is a screen's text with all whitespace removed, and the row
// and byte offset each remaining byte came from.
type squeezedScreen struct {
	rows []string
	text string
	from []screenPos
}

type screenPos struct{ row, off int }

func squeezeScreen(output string) squeezedScreen {
	s := squeezedScreen{rows: strings.Split(output, "\n")}
	var text strings.Builder
	for r, row := range s.rows {
		for off := 0; off < len(row); {
			c, size := utf8.DecodeRuneInString(row[off:])
			if !unicode.IsSpace(c) {
				text.WriteString(row[off : off+size])
				for k := range size {
					s.from = append(s.from, screenPos{r, off + k})
				}
			}
			off += size
		}
	}
	s.text = text.String()
	return s
}

// find is the logical line around needle: the text before it on its first row,
// the needle as declared, and the rest of its last row.
func (s squeezedScreen) find(needle string) (string, bool) {
	n := strings.Join(strings.Fields(needle), "")
	at := strings.Index(s.text, n)
	if n == "" || at < 0 {
		return "", false
	}
	first, last := s.from[at], s.from[at+len(n)-1]
	return strings.TrimSpace(s.rows[first.row][:first.off] + needle + s.rows[last.row][last.off+1:]), true
}
