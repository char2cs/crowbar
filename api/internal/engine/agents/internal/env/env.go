// Package env manipulates process environment slices.
//
// It is pure and dependency-free so both the spawn planner and the probe runner
// can share it without either importing the other.
package env

import "strings"

// Clear returns env with every named variable removed. A descriptor names these
// so a hosted CLI does not inherit markers from the Crowbar process that spawned
// it — a nested-session flag inherited by a child changes how that CLI behaves.
func Clear(environ, names []string) []string {
	if len(names) == 0 {
		return append([]string{}, environ...)
	}
	drop := make(map[string]struct{}, len(names))
	for _, n := range names {
		drop[n] = struct{}{}
	}
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		if _, skip := drop[name]; skip {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// Replace sets name to value, removing any prior entries so the child sees one
// unambiguous value rather than relying on last-wins.
func Replace(environ []string, name, value string) []string {
	prefix := name + "="
	out := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		if !strings.HasPrefix(entry, prefix) {
			out = append(out, entry)
		}
	}
	return append(out, prefix+value)
}

// Lookup returns the value of name in environ, searching from the end so a later
// entry wins — matching how exec resolves duplicates.
func Lookup(environ []string, name string) (string, bool) {
	prefix := name + "="
	for i := len(environ) - 1; i >= 0; i-- {
		if strings.HasPrefix(environ[i], prefix) {
			return strings.TrimPrefix(environ[i], prefix), true
		}
	}
	return "", false
}
