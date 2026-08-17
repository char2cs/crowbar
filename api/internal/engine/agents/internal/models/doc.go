// Package models holds the values the agents engine returns.
//
// Like spec, it is pure: no I/O, no sibling imports, no behaviour beyond what a
// value type needs. The engine root re-exports these as its public API, so a
// caller never imports this package directly.
package models
