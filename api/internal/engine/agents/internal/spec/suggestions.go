package spec

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"

// EventSuggestions is what an ask offers beyond allow and deny: the payload's own
// suggestions, or, when it lists none, those declared for the tool asking. The
// options shown and the answer rendered both read it, so an index names one grant.
// An answer is rendered knowing no channel, so each declared map is tried: only the
// one of the payload's own shape resolves in it.
func (d *Descriptor) EventSuggestions(canonical string, decoded map[string]any) []any {
	e, ok := d.Events[canonical]
	if !ok {
		return nil
	}
	maps := []FieldMap{e.Map}
	for _, block := range []*ChannelBlock{e.API, e.Hooks} {
		if block != nil {
			maps = append(maps, block.Map)
		}
	}
	for _, fields := range maps {
		if offered := mapping.List(decoded, fields["suggestions"]); len(offered) > 0 {
			return offered
		}
	}
	declared := e.ToolSuggestions[mapping.String(decoded, e.Map["tool_name"])]
	out := make([]any, len(declared))
	for i, s := range declared {
		out[i] = s
	}
	return out
}

// OfferName is what an offer is called when its event declares no suggestion_type:
// a bare word is itself, an object of one key that key, anything else nothing.
func OfferName(offer any) string {
	switch o := offer.(type) {
	case string:
		return o
	case map[string]any:
		if len(o) == 1 {
			for name := range o {
				return name
			}
		}
	}
	return ""
}
