package spec

// Unified accessors over the v2 and v3 descriptor shapes.
//
// A descriptor declares one shape or the other. Every consumer goes through these, so
// switching which files ship is one line in the loader rather than an edit in every
// parser — and both shapes stay exercised until the v2 files are deleted.

// IsV3 reports whether this descriptor uses the event-centric shape.
func (d *Descriptor) IsV3() bool { return len(d.Events) > 0 }

// EventFields returns the canonical-field map for an inbound or ask event, and whether
// the provider declares it at all. Key-presence IS the capability check.
func (d *Descriptor) EventFields(canonical string) (map[string]string, bool) {
	if d.IsV3() {
		e, ok := d.Events[canonical]
		if !ok {
			return nil, false
		}
		return e.Map, true
	}
	return d.Hooks.Event(canonical)
}

// DeclaredEvents lists every canonical event the provider observes, sorted.
func (d *Descriptor) DeclaredEvents() []string {
	var out []string
	if d.IsV3() {
		for name, e := range d.Events {
			// Outbound events are things Crowbar SENDS; they are not observations.
			if e.Out != "" {
				continue
			}
			out = append(out, name)
		}
	} else {
		for name := range d.Hooks.Events {
			out = append(out, name)
		}
	}
	sortStrings(out)
	return out
}

// AnswerFor returns the answer channel for an ask event: the decision templates, the
// budget, and where a filled-in form is written.
//
// A v3 event marked `answerable: false` reports false here, which is exactly what a
// missing v2 answer block meant — the prompt is visible but a decision reaches nobody.
func (d *Descriptor) AnswerFor(canonical string) (AnswerEventSpec, bool) {
	if !d.IsV3() {
		return d.Answer.Event(canonical)
	}
	e, ok := d.Events[canonical]
	if !ok || e.Ask == "" {
		return AnswerEventSpec{}, false
	}
	if e.Answerable != nil && !*e.Answerable {
		return AnswerEventSpec{}, false
	}
	if len(e.Reply) == 0 {
		return AnswerEventSpec{}, false
	}
	return AnswerEventSpec{
		TimeoutSeconds: e.TimeoutSeconds,
		AnswersInto:    e.AnswersInto,
		Responses:      e.Reply,
	}, true
}

// WireName returns the provider's own name for a canonical event — the hook name or
// the RPC method. Empty for a v2 descriptor, whose wire names live in its config
// injection rather than in the event table.
func (d *Descriptor) WireName(canonical string) string {
	if !d.IsV3() {
		return ""
	}
	name, _ := d.Events[canonical].WireEvent()
	return name
}

// HookFormat is the payload encoding for hook-transport providers.
func (d *Descriptor) HookFormat() string {
	if d.IsV3() {
		return d.Runtime.Hooks.Format
	}
	return d.Hooks.Format
}

// RequiredPayloadFields are the fields whose absence means the payload describes some
// other CLI's conversation, not this one.
func (d *Descriptor) RequiredPayloadFields() []string {
	if d.IsV3() {
		return d.Runtime.Hooks.RequirePayloadFields
	}
	return d.Hooks.RequirePayloadFields
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
