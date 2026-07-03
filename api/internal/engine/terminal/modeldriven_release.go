//go:build !noEmbed

package terminal

// modelDrivenBuildDefault: release builds keep the raw path until the dev
// bake period and the divergence canary pass (spec §3.7).
const modelDrivenBuildDefault = false
