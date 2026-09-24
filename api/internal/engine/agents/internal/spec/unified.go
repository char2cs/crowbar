package spec

// Accessors over the descriptor's event table.
//
// Every consumer goes through these rather than reaching into Events directly, so the
// rules about capability (key-presence) and answerability live in one place.

// EventFields returns the canonical-field map for an inbound or ask event, and whether
// the provider declares it at all. Key-presence IS the capability check.
func (d *Descriptor) EventFields(canonical string) (FieldMap, bool) {
	e, ok := d.Events[canonical]
	if !ok {
		return nil, false
	}
	return e.Map, true
}

// EventFieldsFor is EventFields, selected by the ACTUAL delivery channel: a
// channel-scoped event's own block, or a legacy event's flat map: on every
// channel. ok is false when the provider declares canonical at all but not on
// THIS channel — a channel a provider does not use is absent, never silently
// satisfied by the other channel's shape (design spec 2.1).
func (d *Descriptor) EventFieldsFor(canonical string, channel Channel) (FieldMap, bool) {
	e, ok := d.Events[canonical]
	if !ok {
		return nil, false
	}
	if !e.HasChannelBlocks() {
		return e.Map, true
	}
	block := e.blockFor(channel)
	if block == nil {
		return nil, false
	}
	return block.Map, true
}

// EventWhenFor is the when: discriminator canonical answers to on channel — the
// channel block's own when: for a channel-scoped event, or the legacy flat
// when: (identical on every channel) for one with no blocks, mirroring
// EventFieldsFor's fallback exactly.
//
// It exists because `when:` used to be read only by dispatch.Resolve, whose
// only caller is the API transport: a provider's method name has to be turned
// back into a canonical event, and a sum-type method needs the discriminator to
// say which. A hook relay already carries the canonical name, so the hooks
// channel had nothing to resolve — and that quietly made one wire hook mean
// exactly ONE canonical event. A provider whose single hook carries two
// different facts (claude's Notification: an idle report, or a permission
// prompt) needs the same discriminator on the hooks channel, where it selects
// which deliveries of that hook a canonical event ACCEPTS rather than which
// event a method resolves to. Enforced at hook-parse time (translate/inbound/
// hooks.go's Parse).
func (d *Descriptor) EventWhenFor(canonical string, channel Channel) WhenMap {
	e, ok := d.Events[canonical]
	if !ok {
		return nil
	}
	return e.WhenFor(channel)
}

// EventRequired is the event's required: list — enforced at hook-parse time
// (translate/inbound/hooks.go's Parse) as a hard, named error (design spec
// 2.3, P5).
func (d *Descriptor) EventRequired(canonical string) []string {
	return d.Events[canonical].Required
}

// EventFixtures is the fixtures: list for canonical on channel — the
// channel block's own list for a channel-scoped event, or the legacy flat
// list (same on every channel) for one with no channel blocks, mirroring
// EventFieldsFor's fallback. Parsed and exposed this phase, enforced
// starting P5 (design spec 2.4). Nil for an undeclared event, a
// channel-scoped event with no block for THIS channel, or a legacy event
// that names no fixtures at all.
func (d *Descriptor) EventFixtures(canonical string, channel Channel) []string {
	e, ok := d.Events[canonical]
	if !ok {
		return nil
	}
	if !e.HasChannelBlocks() {
		return e.Fixtures
	}
	block := e.blockFor(channel)
	if block == nil {
		return nil
	}
	return block.Fixtures
}

// EventOwner is the event's declared owner: — api|hooks|either (design spec
// P6b tag 1). Absent answers Either: declaring an owner is opt-in, and a
// delivery must never be dropped on channel liveness alone with no explicit
// owner naming the OTHER channel — see turn/ingest.go's own doc on the
// zero-writer production incident this guards against.
func (d *Descriptor) EventOwner(canonical string) string {
	if o := d.Events[canonical].Owner; o != "" {
		return o
	}
	return OwnerEither
}

// EventSurfaces is the event's surfaces: list (design spec P6b tag 2) — nil
// (absent) means every surface. Distinct from the descriptor-level Surfaces
// block (a provider capability); this is a per-event visibility gate.
func (d *Descriptor) EventSurfaces(canonical string) []string {
	return d.Events[canonical].Surfaces
}

// EventSteps is the structured `steps:` extra for an event, if it declares one.
// It is separate from EventFields because a flat field map cannot express a LIST
// of {text, status} pairs — see StepsSpec.
func (d *Descriptor) EventSteps(canonical string) *StepsSpec {
	e, ok := d.Events[canonical]
	if !ok {
		return nil
	}
	return e.Steps
}

// DeclaredEvents lists every canonical event the provider observes, sorted.
func (d *Descriptor) DeclaredEvents() []string {
	var out []string
	for name, e := range d.Events {
		// Outbound events are things Crowbar SENDS; they are not observations.
		if !e.Out.Empty() {
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
//
// AnyWireEvent's direction, not the flat e.Ask: a channel-split ask event (see
// ChannelBlock) names its wire method inside api:/hooks:, not at the event's
// own top level — Reply itself stays flat regardless, so this is the only
// change channel-splitting an ask event needs here.
func (d *Descriptor) AnswerFor(canonical string) (AnswerEventSpec, bool) {
	e, ok := d.Events[canonical]
	if !ok {
		return AnswerEventSpec{}, false
	}
	if _, direction := e.AnyWireEvent(); direction != "ask" {
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
	ref, _ := d.Events[canonical].WireEvent()
	return ref.Name()
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
