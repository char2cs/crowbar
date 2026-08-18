package spec

// EffortFallbackKey is the EffortSpec.Available key whose levels apply to any
// model with no list of its own. A descriptor whose whole catalogue is
// model-independent declares this key alone.
const EffortFallbackKey = "*"

// EffortSpec declares which reasoning-effort levels a chat may run a model
// under, and how a choice reaches the process.
//
// Available is keyed BY MODEL, where ModelSpec.Available is flat, and the
// difference is real rather than symmetry for its own sake: an effort level is a
// property of a model. Codex's own catalogue reports supported_reasoning_levels
// per model, so a single flat list could only be right for one of them.
//
// The whole block is optional; its absence is the capability's absence.
type EffortSpec struct {
	Available map[string][]string `yaml:"available"`
	Strategy  string              `yaml:"strategy"`
	Apply     []InjectStep        `yaml:"apply"`
}
