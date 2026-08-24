package domain

// AgentProvider is one registered agent provider as Crowbar resolves it: the
// descriptor catalog joined with the global preference table and the install
// probe. It carries the id a client passes back to create/switch a chat, a human
// display name, and an inline SVG icon (fill="currentColor"). Providers are
// global — nothing here is workspace-dependent.
//
// Connected is whether the provider's spawn.cmd resolves to an installed
// executable on PATH (install-only, no auth probe). Enabled is !Disabled from the
// global AgentProviderPreference (a provider with no stored preference defaults to
// enabled). A resolved list is ordered by priority — priority is implicit in the
// slice position, preferenced providers first in saved order, unpreferenced ones
// appended by descriptor id.
//
// MCPEnabled is whether Crowbar registers its own tool surface with this
// provider, and it is a SEPARATE axis from Enabled: a provider with the tools
// switched off still spawns, still fires its hooks and still holds a normal
// chat — only the tools are gone. Like Enabled it is the positive reading of a
// negatively stored flag (see AgentProviderPreference for why the DB stores the
// negative), so a provider with no row reports true.
type AgentProvider struct {
	ID          string
	DisplayName string
	Icon        string
	Connected   bool
	Enabled     bool
	MCPEnabled  bool

	// ModelSelect and EffortSelect are whether this provider's descriptor declares
	// a model / effort catalogue at all. False means the picker does not exist for
	// it — absent UI, never a disabled control implying breakage. Both shipped
	// descriptors declare both today; the flags exist so a third provider that
	// declares neither renders no picker rather than an empty one.
	// Compaction is whether the provider declares a gesture Crowbar can use to ask it
	// to compact its own context. A UI compact control must be gated on this: a
	// button that appears to work and does not is worse than one that is absent.
	Compaction bool

	// Hotswap and HasTerminal are engine.Capabilities.Hotswap/.HasTerminal,
	// carried through unchanged — see there for what each means.
	Hotswap     bool
	HasTerminal bool

	ModelSelect  bool
	EffortSelect bool

	// Models is the declared catalogue, in descriptor order.
	//
	// Efforts is keyed by model id and ALREADY RESOLVED: every entry in Models has
	// a key, plus "" for the provider's own default model. The descriptor's
	// model-independent fallback is applied here rather than at the edge, so a
	// client picks a model and reads Efforts[model] with no fallback rule to
	// implement and no provider knowledge to hardcode.
	//
	// Both are nil, never empty, for a provider that declares no catalogue: the
	// wire reading of that is an omitted field, which says "this picker does not
	// exist" where an empty array would not.
	Models  []string
	Efforts map[string][]string
}
