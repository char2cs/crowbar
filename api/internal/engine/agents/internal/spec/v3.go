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

	TimeoutSeconds int `yaml:"timeout_seconds"`

	// RateLimits is telemetry's structured extra: a field map cannot express a list
	// of named windows.
	RateLimits []RateLimitSpec `yaml:"rate_limits"`
	// AnswersInto is permission's structured extra.
	AnswersInto string `yaml:"answers_into"`
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
// config | mcp | context | resume.
type InjectSpec struct {
	At    string            `yaml:"at"`
	Call  string            `yaml:"call"`
	Send  map[string]string `yaml:"send"`
	Steps []InjectStep      `yaml:"steps"`
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
