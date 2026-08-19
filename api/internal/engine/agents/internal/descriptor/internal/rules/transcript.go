package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

// transcript checks the block that lets Crowbar read the provider's own record
// of a conversation.
//
// A descriptor declaring NOTHING is valid and unchanged — that is the whole
// degradation contract, and refusing an absent block would break every provider
// that predates the field. What is refused is a HALF-declared one: a block naming
// a payload field but no way to recognise a message would silently read nothing
// while looking, in the file, exactly like a working declaration. A capability
// that is off is legible; a capability that is on and inert is not.
type transcript struct{}

func (transcript) Name() string { return "transcript" }

func (r transcript) Check(d *spec.Descriptor) error {
	t := d.Transcript
	if t.PathField == "" && t.Format == "" && len(t.Message.Match) == 0 &&
		t.Message.Blocks == "" && t.Message.Text == "" {
		return nil
	}
	if t.PathField == "" {
		return invalid(d.ID, "transcript: path_field is required")
	}
	if t.Format != "jsonl" {
		return invalid(d.ID, "transcript: unsupported format %q (only \"jsonl\" is read)", t.Format)
	}
	if len(t.Message.Match) == 0 {
		return invalid(d.ID, "transcript: message.match must name at least one field, "+
			"or every line in the file is a message")
	}
	if t.Message.Blocks == "" && t.Message.Text == "" {
		return invalid(d.ID, "transcript: message needs blocks or text")
	}
	if t.Message.Blocks != "" && t.Message.BlockText == "" {
		return invalid(d.ID, "transcript: message.blocks needs message.block_text")
	}
	if t.MaxReadBytes < 0 || t.MaxMessageBytes < 0 {
		return invalid(d.ID, "transcript: byte ceilings cannot be negative")
	}
	return nil
}
