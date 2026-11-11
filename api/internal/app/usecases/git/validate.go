package git

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// resetModes is the allowlist of git reset modes: the mode becomes a `--<mode>` flag, so an open value
// would allow git argument injection.
var resetModes = map[string]struct{}{
	"soft":  {},
	"mixed": {},
	"hard":  {},
	"keep":  {},
	"merge": {},
}

// validateResetMode rejects any reset mode outside the allowlist. It is the
// primary guard for reset, whose mode is a FLAG (not a positional operand) and
// therefore cannot be protected by a `--` end-of-options separator.
func validateResetMode(
	mode string,
) error {
	if _, ok := resetModes[mode]; !ok {
		return fmt.Errorf(
			"%w: reset mode %q is not one of soft, mixed, hard, keep, merge",
			problem.ErrInvalidArgument, mode,
		)
	}
	return nil
}

// validateOperand rejects a non-empty git operand (label names it) that git could read as an option or
// that escapes ref grammar. A leading dash is refused even where `--` exists, since some positionals
// (git rebase --onto) cannot sit behind it; "" means the default.
func validateOperand(
	label string,
	operand string,
) error {
	if operand == "" {
		return nil
	}
	if strings.HasPrefix(operand, "-") {
		return fmt.Errorf(
			"%w: %s %q must not begin with '-'",
			problem.ErrInvalidArgument, label, operand,
		)
	}
	if !isRefShaped(operand) {
		return fmt.Errorf(
			"%w: %s %q is not a valid git ref/revision",
			problem.ErrInvalidArgument, label, operand,
		)
	}
	return nil
}

// isRefShaped is a conservative in-process subset of `git check-ref-format`: it accepts branch names,
// full refs, SHAs and HEAD^/HEAD~3, and rejects forbidden ref characters, shell/pathspec metacharacters
// and whitespace.
func isRefShaped(
	s string,
) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		// Reject ASCII control chars (NUL, newline, tab, …), DEL, and space.
		if r <= ' ' || r == 0x7f {
			return false
		}
		// '^' and '~' are allowed for revision expressions; inert because the engine never invokes a shell.
		switch r {
		case ':', '?', '[', '\\', '*', '"', '\'', '`', '$', '(', ')',
			'{', '}', '<', '>', '|', '&', ';', '!', '#':
			return false
		}
	}
	// Reject the multi-char sequences git check-ref-format forbids.
	if strings.Contains(s, "..") || strings.Contains(s, "@{") {
		return false
	}
	// Reject leading/trailing shapes git forbids.
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, ".") ||
		strings.HasSuffix(s, "/") || strings.HasSuffix(s, ".") ||
		strings.HasSuffix(s, ".lock") || s == "@" {
		return false
	}
	return true
}
