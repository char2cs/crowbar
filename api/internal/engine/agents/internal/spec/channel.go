package spec

// Channel is which wire actually carried one delivery: the api connection or a
// hook relay POST. It is decided at receive time, never inferred from payload shape
// or an event's static Transport, so a foreign hooks payload is always checked.
type Channel string

const (
	ChannelAPI   Channel = "api"
	ChannelHooks Channel = "hooks"
)

// ChannelBlock is one channel's shape for a dual-shape event: wire name (at most one
// of In/Ask), discriminator, field map and the recorded Fixtures that prove it. Reply
// stays on the event: both channels answer in one decision vocabulary.
type ChannelBlock struct {
	In       WireRef  `yaml:"in"`
	Ask      WireRef  `yaml:"ask"`
	When     WhenMap  `yaml:"when"`
	Map      FieldMap `yaml:"map"`
	Fixtures []string `yaml:"fixtures"`
	// Unverified is the explicit, greppable opt-out from fixtures: this block has
	// no recorded real payload yet, and the gap is acknowledged, not invented.
	Unverified bool `yaml:"unverified"`
	// UnverifiedFields names mapped fields of a block that HAS recordings but none
	// resolves them yet: the replay logs those, and fails once one starts to resolve.
	UnverifiedFields []string `yaml:"unverified_fields"`
	// ByWire maps one of this block's Ask/In wire names to literal fields merged into
	// the payload before map: resolves, for providers that encode a fact only in WHICH
	// wire method arrived (e.g. one per permission kind).
	ByWire map[string]map[string]string `yaml:"by_wire"`
}

// HasChannelBlocks reports whether this event uses the channel-split form
// (api:/hooks: sub-blocks) rather than one flat in:/when:/map:.
func (e EventSpec) HasChannelBlocks() bool {
	return e.API != nil || e.Hooks != nil
}

// blockFor returns this event's block for channel, or nil for a legacy event or
// a channel-scoped one declaring nothing on this channel.
func (e EventSpec) blockFor(channel Channel) *ChannelBlock {
	switch channel {
	case ChannelAPI:
		return e.API
	case ChannelHooks:
		return e.Hooks
	default:
		return nil
	}
}

// WireEventFor returns the wire ref and direction this event answers to on channel:
// that channel's block, else the legacy flat In/Out/Ask. A channel-scoped event with
// no block here reports Empty, never borrowing the other channel's shape.
func (e EventSpec) WireEventFor(channel Channel) (ref WireRef, direction string) {
	if block := e.blockFor(channel); block != nil {
		switch {
		case !block.Ask.Empty():
			return block.Ask, "ask"
		case !block.In.Empty():
			return block.In, "in"
		}
		return nil, ""
	}
	if e.HasChannelBlocks() {
		return nil, ""
	}
	return e.WireEvent()
}

// WhenFor is WireEventFor's counterpart for the when: discriminator, with the
// same block-else-legacy rule.
func (e EventSpec) WhenFor(channel Channel) WhenMap {
	if block := e.blockFor(channel); block != nil {
		return block.When
	}
	if e.HasChannelBlocks() {
		return nil
	}
	return e.When
}

// AnyWireEvent returns the first wire ref this event declares in any form, for
// validation that only asks whether it names something.
func (e EventSpec) AnyWireEvent() (ref WireRef, direction string) {
	if wire, dir := e.WireEvent(); !wire.Empty() {
		return wire, dir
	}
	if wire, dir := e.WireEventFor(ChannelAPI); !wire.Empty() {
		return wire, dir
	}
	return e.WireEventFor(ChannelHooks)
}
