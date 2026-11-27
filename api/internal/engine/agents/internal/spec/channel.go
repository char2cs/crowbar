package spec

type Channel string

const (
	ChannelAPI   Channel = "api"
	ChannelHooks Channel = "hooks"
)

type ChannelBlock struct {
	In               WireRef                      `yaml:"in"`
	Ask              WireRef                      `yaml:"ask"`
	When             WhenMap                      `yaml:"when"`
	Map              FieldMap                     `yaml:"map"`
	Fixtures         []string                     `yaml:"fixtures"`
	Unverified       bool                         `yaml:"unverified"`
	UnverifiedFields []string                     `yaml:"unverified_fields"`
	ByWire           map[string]map[string]string `yaml:"by_wire"`
}

func (e EventSpec) HasChannelBlocks() bool {
	return e.API != nil || e.Hooks != nil
}

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

func (e EventSpec) WhenFor(channel Channel) WhenMap {
	if block := e.blockFor(channel); block != nil {
		return block.When
	}
	if e.HasChannelBlocks() {
		return nil
	}
	return e.When
}

func (e EventSpec) AnyWireEvent() (ref WireRef, direction string) {
	if wire, dir := e.WireEvent(); !wire.Empty() {
		return wire, dir
	}
	if wire, dir := e.WireEventFor(ChannelAPI); !wire.Empty() {
		return wire, dir
	}
	return e.WireEventFor(ChannelHooks)
}
