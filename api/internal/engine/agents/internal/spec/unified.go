package spec

// Accessors over the descriptor's event table.
//
// Every consumer goes through these rather than reaching into Events directly, so the
// rules about capability (key-presence) and answerability live in one place.

// EventFields returns the canonical-field map for an inbound or ask event, and whether
// the provider declares it at all. Key-presence IS the capability check.
func (d *Descriptor) EventFields(canonical string) (map[string]string, bool) {
	e, ok := d.Events[canonical]
	if !ok {
		return nil, false
	}
	return e.Map, true
}

// DeclaredEvents lists every canonical event the provider observes, sorted.
func (d *Descriptor) DeclaredEvents() []string {
	var out []string
	for name, e := range d.Events {
		// Outbound events are things Crowbar SENDS; they are not observations.
		if e.Out != "" {
			continue
		}
		out = append(out, name)
	}
	sortStrings(out)
	return out
}

// AnswerFor returns the answer channel for an ask event: the decision templates, the
// budget, and where a filled-in form is written.
//
// An event marked `answerable: false` reports false here: the prompt is visible but a
// decision would reach nobody, which is the case for codex permissions.
func (d *Descriptor) AnswerFor(canonical string) (AnswerEventSpec, bool) {
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
// the RPC method.
func (d *Descriptor) WireName(canonical string) string {
	name, _ := d.Events[canonical].WireEvent()
	return name
}

// HookFormat is the payload encoding for hook-transport providers.
func (d *Descriptor) HookFormat() string { return d.Runtime.Hooks.Format }

// RequiredPayloadFields are the fields whose absence means the payload describes some
// other CLI's conversation, not this one.
func (d *Descriptor) RequiredPayloadFields() []string {
	return d.Runtime.Hooks.RequirePayloadFields
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
