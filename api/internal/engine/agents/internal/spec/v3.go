package spec

// The v3 descriptor shape: the EVENT is the unit, and transport is a property of an
// event rather than of the provider.

// VersionRange bounds the provider protocol a descriptor was written against.
// A provider's api can be experimental, its method names moving; without this
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

	// Hotswap is whether Crowbar's chat and the provider's terminal can be live at
	// once, so a live turn can hand over mid-flight. False on absence, the
	// conservative answer like every capability key.
	Hotswap bool `yaml:"hotswap"`
}

type APISpec struct {
	Protocol string `yaml:"protocol"`
	// Delivery and Format are how Protocol reaches serve: the only pair is
	// stdio + ndjson, one message per line on serve's own stdin and stdout.
	Delivery string `yaml:"delivery"`
	Format   string `yaml:"format"`
	// Serve starts the server Crowbar speaks to.
	Serve []string `yaml:"serve"`
	// Attach points a TUI at the SAME conversation, so one session yields both the
	// structured protocol and the terminal pane with no screen scraping.
	Attach    []string      `yaml:"attach"`
	Handshake HandshakeSpec `yaml:"handshake"`
}

// HandshakeSpec is what opens a connection: Call with Params, answered, then
// the Notify notification when one is declared.
type HandshakeSpec struct {
	Call   string         `yaml:"call"`
	Params map[string]any `yaml:"params"`
	Notify string         `yaml:"notify"`
}

const (
	APIDeliveryStdio = "stdio"
	APIFormatNDJSON  = "ndjson"
)

type HooksWire struct {
	Format   string `yaml:"format"`
	Delivery string `yaml:"delivery"`
	// RequirePayloadFields are the fields whose absence means the payload describes
	// some other CLI's conversation, not this one.
	RequirePayloadFields []string `yaml:"require_payload_fields"`
}

// EventSpec is one conversational fact. Exactly one of In/Out/Ask names the wire event.
type EventSpec struct {
	In  WireRef `yaml:"in"`  // they tell us
	Out WireRef `yaml:"out"` // we tell them
	Ask WireRef `yaml:"ask"` // they block on our reply

	// Transport overrides RuntimeSpec.Transport for this event alone. This is the
	// whole mechanism behind a MIXED provider — API for turns, hooks for permissions.
	Transport string `yaml:"transport"`

	// Surfaces names which views (chat|terminal) this event is ingested on; nil
	// means every surface. A per-EVENT gate checked at ingest, distinct from the
	// descriptor-level Surfaces capability block that shares its YAML key.
	Surfaces []string `yaml:"surfaces"`

	// When selects among events sharing one wire event, by discriminator: a wire
	// event whose payload is a sum type can serve several canonical events.
	When WhenMap `yaml:"when"`

	// Map pulls canonical fields out of an inbound payload.
	Map FieldMap `yaml:"map"`

	// Required names the fields whose absence is a hard, per-channel parse error.
	Required []string `yaml:"required"`

	// Fixtures is ChannelBlock.Fixtures for the legacy flat form (an event with
	// no api:/hooks: blocks).
	Fixtures []string `yaml:"fixtures"`

	// Unverified is ChannelBlock.Unverified for the legacy flat form.
	Unverified bool `yaml:"unverified"`
	// UnverifiedFields is ChannelBlock.UnverifiedFields's counterpart for it.
	UnverifiedFields []string `yaml:"unverified_fields"`

	// API/Hooks are per-CHANNEL blocks, since one canonical event can arrive shaped
	// differently on each wire. Nil for a legacy event, whose flat In/When/Map answers
	// on every channel; mixing the two forms is refused at load.
	API   *ChannelBlock `yaml:"api"`
	Hooks *ChannelBlock `yaml:"hooks"`
	// Send builds an outbound payload.
	Send map[string]string `yaml:"send"`
	// Reply holds one template per decision the event accepts.
	Reply map[string]string `yaml:"reply"`
	// Answerable false marks an ask: Crowbar can SEE but not answer (the human uses the
	// terminal). Declared, not inferred from an empty reply, which could be a bug.
	Answerable *bool `yaml:"answerable"`

	TimeoutSeconds int `yaml:"timeout_seconds"`

	// RateLimits is telemetry's structured extra: a field map cannot express a list
	// of named windows.
	RateLimits []RateLimitSpec `yaml:"rate_limits"`
	// AnswersInto is permission's structured extra.
	AnswersInto string `yaml:"answers_into"`
	// ToolSuggestions is permission's other extra: per tool, the grants its own
	// dialog offers when the payload lists none (map's suggestions).
	ToolSuggestions map[string][]map[string]any `yaml:"tool_suggestions"`
	// Steps is plan_update's structured extra: a field map cannot express a LIST
	// of {text, status} pairs.
	Steps *StepsSpec `yaml:"steps"`
	// StatusMap translates provider terminal words into Crowbar's canonical
	// component statuses. Unknown values pass through unchanged.
	StatusMap map[string]string `yaml:"status_map"`
	KindMap   map[string]string `yaml:"kind_map"`
	// ShellKindMap turns shell_kind, the provider's word for what a call started,
	// into shell or monitor.
	ShellKindMap map[string]string `yaml:"shell_kind_map"`
	Locations    *LocationsSpec    `yaml:"locations"`
	// DiffFiles maps a tool call's own changed-file list into one preview for
	// that call. It is separate from the turn-wide diff snapshot.
	DiffFiles *DiffFilesSpec `yaml:"diff_files"`
	// Patch is tool_post's structured extra: the provider's own unified-diff hunks
	// for the change a tool made, with the real line numbers.
	Patch *PatchSpec `yaml:"patch"`

	// Fresh/Resume/Action replace Out/Send for an api event that must first ESTABLISH
	// a session: Fresh with no session id, Resume when this connection has not loaded
	// a known one, then Action. They allow nested params and responses feeding calls.
	Fresh  []CallStep `yaml:"fresh"`
	Resume []CallStep `yaml:"resume"`
	Action []CallStep `yaml:"action"`
}

// CallStep is one RPC call in a Fresh/Resume/Action sequence. Send is a YAML tree
// whose every string leaf is a {placeholder} template; Capture pulls named values out
// of the JSON response, by Map's path grammar, for later steps and the caller.
type CallStep struct {
	Call    string            `yaml:"call"`
	Send    map[string]any    `yaml:"send"`
	Capture map[string]string `yaml:"capture"`
}

// StepsSpec maps a provider's plan array onto Crowbar's step vocabulary. StatusMap
// translates provider statuses into pending/active/done, keeping their words out of
// Go; an unmapped status passes through unchanged.
type StepsSpec struct {
	// Items is the path to the array itself.
	Items string `yaml:"items"`
	// Text and Status are paths WITHIN one element.
	Text      string            `yaml:"text"`
	Status    string            `yaml:"status"`
	StatusMap map[string]string `yaml:"status_map"`
}

type LocationsSpec struct {
	Items string `yaml:"items"`
	Path  string `yaml:"path"`
	Line  string `yaml:"line"`
}

type DiffFilesSpec struct {
	Items string `yaml:"items"`
	Path  string `yaml:"path"`
	Diff  string `yaml:"diff"`
	// Kind is the path within a change to its kind word; Whole maps the kind words
	// whose diff is the whole file rather than a hunk to DiffAdded or DiffDeleted.
	Kind  string            `yaml:"kind"`
	Whole map[string]string `yaml:"whole"`
}

// Whole-file kinds a DiffFilesSpec.Whole maps a provider's kind word onto.
const (
	DiffAdded   = "added"
	DiffDeleted = "deleted"
)

// PatchSpec maps a provider's hunk array onto Crowbar's unified-diff vocabulary.
// Items is the path to the array; the rest are paths WITHIN one hunk, where
// Lines holds unified-diff lines (" ctx", "-del", "+add").
type PatchSpec struct {
	Items    string `yaml:"items"`
	OldStart string `yaml:"old_start"`
	NewStart string `yaml:"new_start"`
	Lines    string `yaml:"lines"`
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
// config | mcp | context | resume. Send is a tree like CallStep's, so an injected
// item can be a nested object.
type InjectSpec struct {
	At    string         `yaml:"at"`
	Call  string         `yaml:"call"`
	Send  map[string]any `yaml:"send"`
	Steps InjectSteps    `yaml:"steps"`
}

// TransportFor returns the transport an event uses: its own if it declares one, the
// runtime default otherwise.
func (d *Descriptor) TransportFor(event string) string {
	if e, ok := d.Events[event]; ok && e.Transport != "" {
		return e.Transport
	}
	return d.Runtime.Transport
}

// WireEvent returns the wire name — or, for a provider that namespaces one canonical
// fact across several methods, every name it answers to — and which direction
// declared them.
func (e EventSpec) WireEvent() (ref WireRef, direction string) {
	switch {
	case !e.In.Empty():
		return e.In, "in"
	case !e.Out.Empty():
		return e.Out, "out"
	case !e.Ask.Empty():
		return e.Ask, "ask"
	}
	return nil, ""
}
