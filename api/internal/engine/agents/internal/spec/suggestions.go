package spec

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"

// EventSuggestions is what an ask offers beyond allow and deny: the payload's own
// suggestions, or, when it lists none, those declared for the tool asking. The
// options shown and the answer rendered both read it, so an index names one grant.
func (d *Descriptor) EventSuggestions(canonical string, decoded map[string]any) []map[string]any {
	e, ok := d.Events[canonical]
	if !ok {
		return nil
	}
	fields, _ := d.EventFields(canonical)
	if offered := mapping.Objects(decoded, fields["suggestions"]); len(offered) > 0 {
		return offered
	}
	return e.ToolSuggestions[mapping.String(decoded, fields["tool_name"])]
}
