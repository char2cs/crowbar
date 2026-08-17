// Package spec holds the parsed shape of a provider descriptor: the YAML the
// agents engine reads, and nothing else.
//
// It is the engine's pure layer. It performs no I/O, imports no sibling package,
// and contains no behaviour beyond what YAML decoding requires. Every other
// package under internal/ may depend on it; it depends on none of them, which is
// what makes the engine's dependency graph acyclic by construction rather than
// by convention.
package spec
