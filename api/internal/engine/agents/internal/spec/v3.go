package spec

// The v3 descriptor shape: the EVENT is the unit, and transport is a property of an
// event rather than of the provider.
//
// v2's fields (Hooks, Answer, Presentation, TerminalPrompts, …) still exist on
// Descriptor and both shapes coexist until the v2 consumers are ported. A file
// declares one or the other, never both.

// VersionRange bounds the provider protocol a descriptor was written against.
// Codex's app-server is flagged experimental and its method names move; without this
// a rename surfaces as a runtime mystery instead of a startup failure.
type VersionRange struct {
	Min string `yaml:"min"`
	Max string `yaml:"max"`
}

// RuntimeSpec is how the process is started and spoken to.
type RuntimeSpec struct {
	// Transport is the default for every event that declares none: hooks | api | oneshot.
	Transport string    `yaml:"transport"`
	API       APISpec   `yaml:"api"`
	Hooks     HooksWire `yaml:"hooks"`
	Spawn     SpawnSpec `yaml:"spawn"`

	// Hotswap is whether this provider's TWO faces — Crowbar's chat and the
	// provider's own terminal — can be live at the same instant, so a live turn
	// can hand over mid-flight instead of blocking the switch until it ends.
	// Defaults false on absence: a descriptor that has not thought about this
	// gets the conservative answer, matching every other capability key.
	Hotswap bool `yaml:"hotswap"`
}

type APISpec struct {
	Protocol string `yaml:"protocol"`
	// Serve starts the server Crowbar speaks to.
	Serve []string `yaml:"serve"`
	// Attach points a TUI at the SAME conversation, so one session yields both the
	// structured protocol and the terminal pane with no screen scraping.
	Attach    []string          `yaml:"attach"`
	Handshake map[string]string `yaml:"handshake"`
}

type HooksWire struct {
	Format   string `yaml:"format"`
	Delivery string `yaml:"delivery"`
	// RequirePayloadFields are the fields whose absence means the payload describes
	// some other CLI's conversation, not this one.
	RequirePayloadFields []string `yaml:"require_payload_fields"`
}

// EventSpec is one conversational fact. Exactly one of In/Out/Ask names the wire event.
type EventSpec struct {
	In  string `yaml:"in"`  // they tell us
	Out string `yaml:"out"` // we tell them
	Ask string `yaml:"ask"` // they block on our reply

	// Transport overrides RuntimeSpec.Transport for this event alone. This is the
	// whole mechanism behind a MIXED provider — API for turns, hooks for permissions.
	Transport string `yaml:"transport"`

	// When selects among events sharing one wire event, by discriminator. Codex's
	// `item` is a sum type and item/started serves three canonical events.
	When map[string]string `yaml:"when"`

	// Map pulls canonical fields out of an inbound payload.
	Map map[string]string `yaml:"map"`
	// Send builds an outbound payload.
	Send map[string]string `yaml:"send"`
	// Reply holds one template per decision the event accepts.
	Reply map[string]string `yaml:"reply"`
	// Risk classifies this event's tool calls into engineagents.RiskTier
	// buckets, keyed by tier name ("read-only", "standard", "sensitive",
	// "internal") to a list of tool-name patterns (a trailing "*" matches a
	// prefix; anything else must match exactly). Only meaningful for the
	// permission event today. A tool matching no pattern in any bucket, or a
	// descriptor declaring no risk: block at all, classifies as "sensitive" —
	// see models.RiskSensitive.
	Risk map[string][]string `yaml:"risk"`
	// Answerable false marks an ask: event Crowbar can SEE but not answer — the
	// provider declares no response template, so the human answers in the terminal
	// instead. Declared rather than inferred from an empty reply: "no templates" and
	// "not answerable" look identical, and one of them is a bug.
	Answerable *bool `yaml:"answerable"`

	TimeoutSeconds int `yaml:"timeout_seconds"`

	// RateLimits is telemetry's structured extra: a field map cannot express a list
	// of named windows.
	RateLimits []RateLimitSpec `yaml:"rate_limits"`
	// AnswersInto is permission's structured extra.
	AnswersInto string `yaml:"answers_into"`

	// Fresh/Resume/Action are the alternative to Out/Send for an api-transport
	// event that must first ESTABLISH a session before it can act — codex's
	// `turn/start` needs a `thread/start` (nothing known yet) or a
	// `thread/resume` (a session id is already known, e.g. from a prior life of
	// this chat, but THIS connection has never itself loaded it) ahead of it,
	// and neither the field-per-string Send shape nor a single Out call can
	// express a call whose params are typed/nested (thread/start's `input` is
	// an array of typed content blocks) or whose response feeds the next call
	// (thread/start's id becomes turn/start's threadId).
	//
	// Fresh runs when no session id is known yet; Resume runs when one is known
	// but this connection has not itself established it; Action always runs
	// last. Mutually exclusive with Out/Send, which remain the single flat-call
	// outbound shape for an event that needs neither (interrupt, compact_start).
	Fresh  []CallStep `yaml:"fresh"`
	Resume []CallStep `yaml:"resume"`
	Action []CallStep `yaml:"action"`
}

// CallStep is one RPC call in a Fresh/Resume/Action sequence. Send is an
// arbitrary (YAML-shaped) tree, not a flat field map — every string leaf,
// however deeply nested, is a {placeholder} template — so a typed/nested
// payload (an array of typed content blocks, a nested object) is just normal
// YAML rather than something Go code assembles. Capture pulls named fields out
// of the call's JSON response, by the SAME dotted-path grammar Map already
// uses, into the caller's own values for later steps (and the caller) to read
// — thread/start's response has no field literally called "session_id"; a
// descriptor names whatever dotted path it actually returned.
type CallStep struct {
	Call    string            `yaml:"call"`
	Send    map[string]any    `yaml:"send"`
	Capture map[string]string `yaml:"capture"`
}

type RateLimitSpec struct {
	ID          string `yaml:"id"`
	Label       string `yaml:"label"`
	UsedPercent string `yaml:"used_percent"`
	ResetsAt    string `yaml:"resets_at"`
}

// CallSpec is a catalogue Crowbar reads on demand.
type CallSpec struct {
	Call string            `yaml:"call"`
	Map  map[string]string `yaml:"map"`
}

// InjectSpec is one setup action, keyed by the lifecycle moment it happens at:
// config | mcp | context | resume. Send is a TREE like CallStep's own — codex's
// thread/inject_items needs a nested Responses API item, not a flat
// {field: "{value}"} shape.
type InjectSpec struct {
	At    string         `yaml:"at"`
	Call  string         `yaml:"call"`
	Send  map[string]any `yaml:"send"`
	Steps []InjectStep   `yaml:"steps"`
}

// TransportFor returns the transport an event uses: its own if it declares one, the
// runtime default otherwise.
func (d *Descriptor) TransportFor(event string) string {
	if e, ok := d.Events[event]; ok && e.Transport != "" {
		return e.Transport
	}
	return d.Runtime.Transport
}

// WireEvent returns the wire name and which direction declared it.
func (e EventSpec) WireEvent() (name, direction string) {
	switch {
	case e.In != "":
		return e.In, "in"
	case e.Out != "":
		return e.Out, "out"
	case e.Ask != "":
		return e.Ask, "ask"
	}
	return "", ""
}
