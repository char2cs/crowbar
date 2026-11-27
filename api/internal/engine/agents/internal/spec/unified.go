package spec

func (d *Descriptor) EventFields(canonical string) (FieldMap, bool) {
	e, ok := d.Events[canonical]
	if !ok {
		return nil, false
	}
	return e.Map, true
}

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

func (d *Descriptor) EventWhenFor(canonical string, channel Channel) WhenMap {
	e, ok := d.Events[canonical]
	if !ok {
		return nil
	}
	return e.WhenFor(channel)
}

func (d *Descriptor) EventRequired(canonical string) []string {
	return d.Events[canonical].Required
}

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

func (d *Descriptor) EventSurfaces(canonical string) []string {
	return d.Events[canonical].Surfaces
}

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

func (d *Descriptor) EventShellKindMap(canonical string) map[string]string {
	return d.Events[canonical].ShellKindMap
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

func (d *Descriptor) DeclaredEvents() []string {
	var out []string
	for name, e := range d.Events {
		if !e.Out.Empty() {
			continue
		}
		out = append(out, name)
	}
	sortStrings(out)
	return out
}

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

func (d *Descriptor) WireName(canonical string) string {
	ref, _ := d.Events[canonical].WireEvent()
	return ref.Name()
}

func (d *Descriptor) HookFormat() string { return d.Runtime.Hooks.Format }

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
