package agents

import (
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func (a *agent) SessionAssignArg() (string, bool) {
	if a.spec.Session.Assign == nil || a.spec.Session.Assign.Arg == "" {
		return "", false
	}
	return a.spec.Session.Assign.Arg, true
}

func (a *agent) SessionPinsRelay() bool {
	ev := a.spec.Events["prompt"]
	for _, steps := range [][]spec.CallStep{ev.Fresh, ev.Resume} {
		for _, step := range steps {
			raw, err := json.Marshal(step.Send)
			if err == nil && strings.Contains(string(raw), "{relay_id}") {
				return true
			}
		}
	}
	return false
}

func (a *agent) SessionPersists() string { return a.spec.Session.Persists }

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

func (s squeezedScreen) find(needle string) (string, bool) {
	n := strings.Join(strings.Fields(needle), "")
	at := strings.Index(s.text, n)
	if n == "" || at < 0 {
		return "", false
	}
	first, last := s.from[at], s.from[at+len(n)-1]
	return strings.TrimSpace(s.rows[first.row][:first.off] + needle + s.rows[last.row][last.off+1:]), true
}

func (a *agent) ResumeArg() (string, bool) {
	if a.spec.Session.Resume == nil || a.spec.Session.Resume.Arg == "" {
		return "", false
	}
	return a.spec.Session.Resume.Arg, true
}
