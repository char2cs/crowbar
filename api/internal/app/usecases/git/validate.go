package git

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

var resetModes = map[string]struct{}{
	"soft":  {},
	"mixed": {},
	"hard":  {},
	"keep":  {},
	"merge": {},
}

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

func isRefShaped(
	s string,
) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r <= ' ' || r == 0x7f {
			return false
		}
		switch r {
		case ':', '?', '[', '\\', '*', '"', '\'', '`', '$', '(', ')',
			'{', '}', '<', '>', '|', '&', ';', '!', '#':
			return false
		}
	}
	if strings.Contains(s, "..") || strings.Contains(s, "@{") {
		return false
	}
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, ".") ||
		strings.HasSuffix(s, "/") || strings.HasSuffix(s, ".") ||
		strings.HasSuffix(s, ".lock") || s == "@" {
		return false
	}
	return true
}
