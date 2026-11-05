package spec

// Accessors over the descriptor's event table. Consumers use these rather than
// Events directly, so capability (key-presence) and answerability live in one place.

// EventFields returns the canonical-field map for an inbound or ask event, and whether
// the provider declares it at all. Key-presence IS the capability check.
func (d *Descriptor) EventFields(canonical string) (FieldMap, bool) {
	e, ok := d.Events[canonical]
	if !ok {
		return nil, false
	}
	return e.Map, true
}

// EventFieldsFor is EventFields for the ACTUAL delivery channel: the event's block,
// or a legacy flat map: on every channel. ok is false when canonical is declared but
// not on this channel; the other channel's shape never stands in.
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

// EventWhenFor is canonical's when: discriminator on channel, with EventFieldsFor's
// fallback. On the hooks channel it selects which deliveries of one wire hook the
// event ACCEPTS, since a single hook can carry several distinct facts.
func (d *Descriptor) EventWhenFor(canonical string, channel Channel) WhenMap {
	e, ok := d.Events[canonical]
	if !ok {
		return nil
	}
	return e.WhenFor(channel)
}

// EventRequired is the event's required: list, enforced at parse time as a
// hard, named error.
func (d *Descriptor) EventRequired(canonical string) []string {
	return d.Events[canonical].Required
}

// EventFixtures is the fixtures: list for canonical on channel, with
// EventFieldsFor's block-else-legacy fallback; nil when none applies.
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

// EventSurfaces is the event's surfaces: visibility gate; nil means every surface.
// Distinct from the descriptor-level Surfaces capability block.
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

func (d *Descriptor) EventStatusMap(canonical string) map[string]string {
	return d.Events[canonical].StatusMap
}

func (d *Descriptor) EventKindMap(canonical string) map[string]string {
	return d.Events[canonical].KindMap
}

func (d *Descriptor) EventLocations(canonical string) *LocationsSpec {
	return d.Events[canonical].Locations
}

func (d *Descriptor) EventDiffFiles(canonical string) *DiffFilesSpec {
	return d.Events[canonical].DiffFiles
}

func (d *Descriptor) EventPatch(canonical string) *PatchSpec {
	return d.Events[canonical].Patch
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

// AnswerFor returns an ask event's answer channel (decision templates, budget, form
// target); false for `answerable: false`. Direction comes from AnyWireEvent because a
// channel-split ask names its wire method inside a block.
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
