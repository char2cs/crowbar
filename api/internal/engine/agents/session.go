package agents

import "strings"

func (a *agent) SessionAssignArg() (string, bool) {
	if a.spec.Session.Assign == nil || a.spec.Session.Assign.Arg == "" {
		return "", false
	}
	return a.spec.Session.Assign.Arg, true
}

// RefusedResume is refused only for the declared exit code AND a line naming
// one of the descriptor's needles: any other exit says nothing about the session.
func (a *agent) RefusedResume(exitCode int, output string) (string, bool) {
	r := a.spec.Session.Refused
	if r == nil || exitCode != r.ExitCode {
		return "", false
	}
	for line := range strings.Lines(output) {
		for _, needle := range r.Needles {
			if needle != "" && strings.Contains(line, needle) {
				return strings.TrimSpace(line), true
			}
		}
	}
	return "", false
}
