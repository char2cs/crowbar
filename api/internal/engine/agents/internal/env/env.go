package env

import "strings"

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

func Lookup(environ []string, name string) (string, bool) {
	prefix := name + "="
	for i := len(environ) - 1; i >= 0; i-- {
		if strings.HasPrefix(environ[i], prefix) {
			return strings.TrimPrefix(environ[i], prefix), true
		}
	}
	return "", false
}
