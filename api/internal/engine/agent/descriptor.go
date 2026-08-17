package agent

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Descriptor holds only fields the engine consumes. Every field here is
// load-bearing; provider-specific shapes (hook-config layout, native event
// names) live in the descriptor's literal write-side content, never here.
type Descriptor struct {
	ID string `yaml:"id"`
	// DisplayName and Icon are the ONLY display-only fields on the descriptor
	// (the "every field is load-bearing" invariant's documented carve-out): they
	// are surfaced to the FE by GET .../agent/providers (dto.AgentProviderDTO) to
	// label and glyph the provider, and never influence spawn/hook behaviour.
	// Both are optional — Validate does not require them.
	DisplayName string `yaml:"display_name"`
	Icon        string `yaml:"icon"`
	Spawn       struct {
		Cmd                 string   `yaml:"cmd"`
		InteractiveRequired bool     `yaml:"interactive_required"`
		ForbidFlags         []string `yaml:"forbid_flags"`
		Args                []string `yaml:"args"`
		Env                 struct {
			Clear []string `yaml:"clear"`
		} `yaml:"env"`
	} `yaml:"spawn"`
	Session struct {
		Resume *ArgSpec `yaml:"resume"`
	} `yaml:"session"`
	ConfigInjection []InjectStep `yaml:"config_injection"`
	// MCPInject registers Crowbar's own tool surface with this CLI, and is a
	// SEPARATE list from ConfigInjection because it is the one group of steps that
	// is conditional: the user can switch the tool surface off per provider
	// (AgentProviderPreference.MCPDisabled) while the CLI still spawns and its
	// hooks still fire.
	//
	// Naming the group is the only mechanism that can express that. Filtering
	// ConfigInjection by template token was the obvious alternative and is wrong:
	// {runner_token} appears in exactly one of codex's four MCP steps, so
	// mcp_servers.crowbar.command, .env_vars and .default_tools_approval_mode
	// would all survive the filter and register a server with no arguments — a
	// half-configured tool surface, which is worse than either state the switch
	// is meant to choose between.
	//
	// Validate does NOT require it. A descriptor that declares none simply
	// registers no tools, which is the correct reading for a third-party CLI
	// Crowbar has no MCP wiring for.
	MCPInject []InjectStep `yaml:"mcp_injection"`
	Hooks     HookSpec     `yaml:"hooks"`
	// ContextInject delivers Crowbar's {context} document — the chat-title
	// instruction and/or the handed-off conversation, composed by the usecase —
	// to a CLI starting a FRESH provider session. Every provider must deliver it
	// through a channel the model reads WITHOUT being given a turn: claude's
	// --append-system-prompt, codex's `-c developer_instructions=` (verified
	// against codex 0.139.0: the CLI comes up with an empty composer and answers
	// from the injected context when the user first types). A positional prompt
	// is NOT such a channel — it IS the user's first message, so the CLI answers
	// the handoff instead of waiting for the user.
	ContextInject []InjectStep `yaml:"context_inject"`
	// ResumeContextInject delivers {context} to a CLI being resumed into its OWN
	// prior native session (session.resume), where {context} is only the GAP —
	// what happened under other providers while this one was away.
	//
	// It is a SEPARATE step list because a resumed session does not necessarily
	// accept the same channel a fresh one does. Verified against codex 0.139.0:
	// `codex resume` rebuilds the conversation from its rollout file, which never
	// records developer instructions — a new `-c developer_instructions=` is
	// silently ignored, AGENTS.md is not re-read, and `codex fork` behaves the
	// same. The ONLY channel that reaches a resumed codex is a user message, so
	// codex declares a positional here and the wrapper prose (config.yaml's
	// handoff_resume_wrapper) tells it to acknowledge and wait rather than act.
	// claude has no such limit: --resume honours a fresh --append-system-prompt,
	// so it declares the same silent channel it uses when fresh.
	ResumeContextInject []InjectStep `yaml:"resume_context_inject"`
	// Presentation contains optional, declarative adapters for the React chat
	// presentation. It never changes whether the provider can run in its native
	// terminal: an absent capability means terminal-only for that operation.
	Presentation PresentationSpec `yaml:"presentation"`
}

type ArgSpec struct {
	Arg string `yaml:"arg"`
}

// HookSpec is the read side: how to parse this CLI's hook payloads (format) and
// where each Crowbar vocabulary field sits inside each canonical event's
// payload (events[canonical][vocab] = dotted path).
type HookSpec struct {
	Format string                       `yaml:"format"`
	Events map[string]map[string]string `yaml:"events"`
	// RequirePayloadFields names payload paths that EVERY hook of the CLI's own
	// user-facing conversation carries. A payload missing any of them is not a
	// conversation Crowbar hosts and is DROPPED before the reducer ever sees it.
	//
	// It exists because a CLI's hooks are not only its user's. A modern CLI runs
	// work of its own through the very hook commands Crowbar injected — same
	// process, same config — and those runs look, to a hook listener, exactly like
	// the user opening a new conversation. Verified live against codex 0.146.0 with
	// `[features] memories = true`: its Memory Writing Agent runs as an INTERNAL
	// session that fires SessionStart / UserPromptSubmit / Stop / SessionEnd with a
	// fresh session id and `source: startup` — byte-identical to a real /new — so
	// nothing in the move vocabulary can tell them apart (which is exactly why
	// Decide must not try; see reducer.go).
	//
	// What DOES tell them apart is that they are not conversations at all: they have
	// no rollout on disk (`transcript_path: null`). That is the right thing to test
	// rather than a convenient one — a session with no transcript is a session
	// Crowbar cannot resume and cannot hand off, so hosting it was never possible.
	//
	// The guard is per-PAYLOAD, not per-event, and it has to be: dropping only the
	// announcement would leave the internal session's user_prompt and turn_stop to
	// route by the runner's placement into the USER'S chat — the consolidation
	// prompt landing in their ledger as something they said, and its title derived
	// from it.
	//
	// A provider that names nothing here keeps exactly its previous behaviour.
	RequirePayloadFields []string `yaml:"require_payload_fields"`
}

// InjectStep is one declarative injection verb, e.g. `- pass_arg: {arg: --settings, value: x}`.
// The YAML is a single-key map; the key is the verb, the value is its args.
type InjectStep struct {
	Verb string
	Args map[string]any
}

func (s *InjectStep) UnmarshalYAML(value *yaml.Node) error {
	var m map[string]map[string]any
	if err := value.Decode(&m); err != nil {
		return fmt.Errorf("agent: inject step decode: %w", err)
	}
	if len(m) != 1 {
		return fmt.Errorf("agent: inject step must have exactly one verb, got %d", len(m))
	}
	for verb, args := range m {
		s.Verb = verb
		s.Args = args
	}
	return nil
}

func LoadDescriptor(data []byte) (*Descriptor, error) {
	var d Descriptor
	if err := yaml.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("agent: descriptor unmarshal: %w", err)
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return &d, nil
}

func (d *Descriptor) Validate() error {
	if d.ID == "" {
		return fmt.Errorf("agent: descriptor missing id")
	}
	if d.Spawn.Cmd == "" {
		return fmt.Errorf("agent: descriptor %q missing spawn.cmd", d.ID)
	}
	if !d.Spawn.InteractiveRequired {
		return fmt.Errorf("agent: descriptor %q must set spawn.interactive_required", d.ID)
	}
	if d.Hooks.Format == "" {
		return fmt.Errorf("agent: descriptor %q missing hooks.format", d.ID)
	}
	if ss := d.Hooks.Events["session_start"]; ss["session_id"] == "" {
		return fmt.Errorf("agent: descriptor %q hooks.events.session_start must map session_id", d.ID)
	}
	if ts := d.Hooks.Events["turn_stop"]; ts["message"] == "" {
		return fmt.Errorf("agent: descriptor %q hooks.events.turn_stop must map message", d.ID)
	}
	if err := d.validatePresentation(); err != nil {
		return err
	}
	return nil
}

// ParsePayload decodes raw hook bytes into a map per the descriptor's declared
// format. An unsupported format is an explicit error (documented boundary).
func (d *Descriptor) ParsePayload(raw []byte) (map[string]any, error) {
	switch d.Hooks.Format {
	case "json":
		if len(raw) == 0 {
			return map[string]any{}, nil
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("agent: descriptor %q parse json payload: %w", d.ID, err)
		}
		return m, nil
	default:
		return nil, fmt.Errorf("agent: descriptor %q unsupported hooks.format %q", d.ID, d.Hooks.Format)
	}
}
