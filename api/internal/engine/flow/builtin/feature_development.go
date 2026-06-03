// Package builtin provides the hardcoded built-in flows for Crowbar.
package builtin

import "github.com/char2cs/crowbar/api/internal/engine/flow"

// FeatureDevelopmentFlow is the built-in feature development flow.
// It is defined in the parent flow package to avoid import cycles; this alias
// provides a stable external reference for callers that import only this package.
var FeatureDevelopmentFlow = flow.FeatureDevelopmentFlow
