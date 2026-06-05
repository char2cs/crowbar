package metadata

import "runtime"

// OsValue is a value that can be overridden per operating system.
type OsValue[T any] struct {
	Default T            `yaml:"default"`
	OS      map[string]T `yaml:"os"`
}

// Resolve returns the OS-specific value for runtime.GOOS if present,
// otherwise returns Default.
func (v OsValue[T]) Resolve() T {
	if v.OS == nil {
		return v.Default
	}
	if val, ok := v.OS[runtime.GOOS]; ok {
		return val
	}
	return v.Default
}
