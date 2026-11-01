package types

import "fmt"

// CLIError is a code-host command that failed. Stderr is what the tool printed,
// which says how to fix it (sign in, a PR already exists), unlike its exit status.
type CLIError struct {
	Tool   string
	Verb   string
	Stderr string
	Err    error
}

func (e *CLIError) Error() string {
	return fmt.Sprintf("%s %s: stderr=%q: %v", e.Tool, e.Verb, e.Stderr, e.Err)
}

func (e *CLIError) Unwrap() error { return e.Err }

// Detail is the tool's own words: the failure as a user reads it.
func (e *CLIError) Detail() string { return e.Stderr }
