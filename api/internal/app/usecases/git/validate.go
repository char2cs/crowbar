package git

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
)

// resetModes is the allowlist of git reset modes the API accepts. The mode is
// turned into a `--<mode>` flag by the engine, so an unconstrained value would
// let a caller inject an arbitrary git option (argument injection → RCE /
// arbitrary file write). Only these well-known modes are permitted.
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
			apperr.ErrInvalidArgument, mode,
		)
	}
	return nil
}

// validateOperand rejects a user-controlled git operand (branch / ref / onto /
// commit) that could be misread by git as a command-line option or that escapes
// git's ref/revision grammar. Defense in depth: even where the engine inserts a
// `--` end-of-options separator before the operand, a leading-dash operand is
// rejected here so it can never become an option if a separator is ever missing
// or infeasible for a given subcommand (e.g. the upstream/branch positionals of
// `git rebase --onto`, which git's grammar will not hide behind `--`).
//
// label names the field for the error message (e.g. "branch", "onto", "ref").
// An empty operand is permitted (callers treat "" as "use the default") and is
// left for the engine/git to handle; the checks only apply to non-empty values.
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
			apperr.ErrInvalidArgument, label, operand,
		)
	}
	if !isRefShaped(operand) {
		return fmt.Errorf(
			"%w: %s %q is not a valid git ref/revision",
			apperr.ErrInvalidArgument, label, operand,
		)
	}
	return nil
}

// isRefShaped applies a conservative subset of `git check-ref-format`
// semantics in-process (no shell-out) to a non-empty operand. It is permissive
// enough to accept every operand the git write usecase legitimately passes —
// branch names (feature/x), full refs (refs/heads/x), abbreviated/full SHAs,
// and the revision forms used by reset (HEAD, HEAD^, HEAD~3) — while rejecting
// the shapes git itself forbids in ref names, plus shell/pathspec metacharacters
// and whitespace. Combined with the leading-dash guard in validateOperand this
// keeps an operand from being read as an option or escaping git's grammar.
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
		// Reject git check-ref-format forbidden chars and shell/pathspec
		// metacharacters. '^' and '~' are intentionally allowed: they are part
		// of legitimate revision expressions (HEAD^, HEAD~3) and are inert to
		// the shell because the engine never invokes a shell.
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
