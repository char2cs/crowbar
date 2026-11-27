package spec

type VersionRange struct {
	Min string `yaml:"min"`
	Max string `yaml:"max"`
}

type RuntimeSpec struct {
	Transport string    `yaml:"transport"`
	API       APISpec   `yaml:"api"`
	Hooks     HooksWire `yaml:"hooks"`
	Spawn     SpawnSpec `yaml:"spawn"`

	Hotswap bool `yaml:"hotswap"`
}

type APISpec struct {
	Protocol  string        `yaml:"protocol"`
	Delivery  string        `yaml:"delivery"`
	Format    string        `yaml:"format"`
	Serve     []string      `yaml:"serve"`
	Attach    []string      `yaml:"attach"`
	Handshake HandshakeSpec `yaml:"handshake"`

	Connect    []string               `yaml:"connect"`
	ConnectMap ConnectMap             `yaml:"connect_map"`
	StaticURL  string                 `yaml:"static_url"`
	Auth       AuthSpec               `yaml:"auth"`
	Requests   map[string]RequestSpec `yaml:"requests"`
	Stream     StreamSpec             `yaml:"stream"`
	Reconnect  ReconnectSpec          `yaml:"reconnect"`
	Remember   []FrameCapture         `yaml:"remember"`
}

const CapturedNamespace = "captured"

type FrameCapture struct {
	In      WireRef           `yaml:"in"`
	When    WhenMap           `yaml:"when"`
	Capture map[string]string `yaml:"capture"`
}

type ConnectMap struct {
	URL        string            `yaml:"url"`
	Protocols  string            `yaml:"protocols"`
	Headers    map[string]string `yaml:"headers"`
	Credential string            `yaml:"credential"`
}

type AuthSpec struct {
	BearerEnv  string `yaml:"bearer_env"`
	BearerPath string `yaml:"bearer_path"`
}

type RequestSpec struct {
	Method   string            `yaml:"method"`
	Path     string            `yaml:"path"`
	Body     map[string]any    `yaml:"body"`
	Capture  map[string]string `yaml:"capture"`
	OKStatus []int             `yaml:"ok_status"`
}

type StreamSpec struct {
	Path string `yaml:"path"`
}

type ReconnectSpec struct {
	Max       int `yaml:"max"`
	BackoffMS int `yaml:"backoff_ms"`
}

type HandshakeSpec struct {
	Call   string         `yaml:"call"`
	Params map[string]any `yaml:"params"`
	Notify string         `yaml:"notify"`
}

const (
	APIDeliveryStdio     = "stdio"
	APIDeliveryWebsocket = "websocket"
	APIDeliveryHTTP      = "http"
	APIFormatNDJSON      = "ndjson"
	APIFormatJSONRPCText = "jsonrpc_text"
	APIFormatSSE         = "sse"
)

type HooksWire struct {
	Format               string   `yaml:"format"`
	Delivery             string   `yaml:"delivery"`
	RequirePayloadFields []string `yaml:"require_payload_fields"`
}

type EventSpec struct {
	In  WireRef `yaml:"in"`
	Out WireRef `yaml:"out"`
	Ask WireRef `yaml:"ask"`

	Transport string `yaml:"transport"`

	Surfaces []string `yaml:"surfaces"`

	When WhenMap `yaml:"when"`

	Also bool `yaml:"also"`

	EndsAtAction bool `yaml:"ends_at_action"`

	Map FieldMap `yaml:"map"`

	Required []string `yaml:"required"`

	Fixtures []string `yaml:"fixtures"`

	Unverified       bool     `yaml:"unverified"`
	UnverifiedFields []string `yaml:"unverified_fields"`

	API          *ChannelBlock     `yaml:"api"`
	Hooks        *ChannelBlock     `yaml:"hooks"`
	Send         map[string]any    `yaml:"send"`
	Reply        map[string]string `yaml:"reply"`
	ReplyRequest string            `yaml:"reply_request"`
	Answerable   *bool             `yaml:"answerable"`

	TimeoutSeconds int `yaml:"timeout_seconds"`

	RateLimits      []RateLimitSpec             `yaml:"rate_limits"`
	AnswersInto     string                      `yaml:"answers_into"`
	ToolSuggestions map[string][]map[string]any `yaml:"tool_suggestions"`
	Steps           *StepsSpec                  `yaml:"steps"`
	StatusMap       map[string]string           `yaml:"status_map"`
	KindMap         map[string]string           `yaml:"kind_map"`
	ShellKindMap    map[string]string           `yaml:"shell_kind_map"`
	Locations       *LocationsSpec              `yaml:"locations"`
	DiffFiles       *DiffFilesSpec              `yaml:"diff_files"`
	Patch           *PatchSpec                  `yaml:"patch"`

	Fresh  []CallStep `yaml:"fresh"`
	Resume []CallStep `yaml:"resume"`
	Action []CallStep `yaml:"action"`
}

type CallStep struct {
	Call     string            `yaml:"call"`
	Send     map[string]any    `yaml:"send"`
	Capture  map[string]string `yaml:"capture"`
	Optional []string          `yaml:"optional"`
}

type StepsSpec struct {
	Items     string            `yaml:"items"`
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
	Items string            `yaml:"items"`
	Path  string            `yaml:"path"`
	Diff  string            `yaml:"diff"`
	Kind  string            `yaml:"kind"`
	Whole map[string]string `yaml:"whole"`
}

const (
	DiffAdded   = "added"
	DiffDeleted = "deleted"
)

type PatchSpec struct {
	Unified  string `yaml:"unified"`
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

type CallSpec struct {
	Call string            `yaml:"call"`
	Map  map[string]string `yaml:"map"`
}

type InjectSpec struct {
	At         string         `yaml:"at"`
	Call       string         `yaml:"call"`
	Send       map[string]any `yaml:"send"`
	Steps      InjectSteps    `yaml:"steps"`
	PromptLead string         `yaml:"prompt_lead"`
}

func (d *Descriptor) TransportFor(event string) string {
	if e, ok := d.Events[event]; ok && e.Transport != "" {
		return e.Transport
	}
	return d.Runtime.Transport
}

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
