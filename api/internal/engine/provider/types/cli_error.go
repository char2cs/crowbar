package types

import "fmt"

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

func (e *CLIError) Detail() string { return e.Stderr }
