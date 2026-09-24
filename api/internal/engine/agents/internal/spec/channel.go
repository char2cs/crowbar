package spec

// Channel is which wire actually carried one delivery: the persistent api
// connection, or a hook relay POST. It is a fact about the DELIVERY, decided
// at receive time by whoever accepted the bytes — never inferred from payload
// shape and never an event's static Transport, which is exactly what let a
// dual-shape event's foreign hooks payload through unchecked (see
// translate/inbound/hooks.go's own doc comment, and docs/plans/
// 2026-09-22-descriptor-channel-split.md 1.1).
type Channel string

const (
	ChannelAPI   Channel = "api"
	ChannelHooks Channel = "hooks"
)

// ChannelBlock is one channel's own shape for a dual-shape event: its own
// wire name, an optional sum-type discriminator, its own field map, and the
// recorded payloads proving it. Fixtures is parsed and exposed
// (Descriptor.EventFixtures) and enforced by
// TestV3Descriptors_ResolveAgainstRecordedTraffic (design spec 2.4, P5) —
// see Unverified for the declared opt-out.
//
// In and Ask are the block's own counterparts of the event-level In/Ask: a
// block declares at most one of them (checked at load, descriptor.
// checkChannelSplit), matching which direction the CANONICAL event is. Reply
// stays on the event, not per block: both of a real dual-shape ask's channels
// answer through the same decision vocabulary (confirmed for codex's own
// permission — see codex.yaml's own comment on its reply: block), and
// AnswerFor/RenderAnswer have no channel input to select a per-block one with.
type ChannelBlock struct {
	In       WireRef  `yaml:"in"`
	Ask      WireRef  `yaml:"ask"`
	When     WhenMap  `yaml:"when"`
	Map      FieldMap `yaml:"map"`
	Fixtures []string `yaml:"fixtures"`
	// Unverified is the explicit, greppable opt-out from the fixtures: hard
	// rule (design spec 2.4): true means this block genuinely has no
	// recorded real payload yet, and that gap is ACKNOWLEDGED rather than
	// invented or silently skipped. See EventSpec.Unverified for the legacy
	// flat form's own counterpart.
	Unverified bool `yaml:"unverified"`
	// ByWire is keyed by ONE of this block's own Ask/In wire names, and its
	// value is literal synthetic payload fields to inject before map:
	// resolves — the generic primitive design spec F1 asks for: a provider
	// that NAMESPACES one canonical fact across several wire methods (codex's
	// permission answers to item/commandExecution/requestApproval for a
	// command and item/fileChange/requestApproval for a patch, with no
	// payload field of its own saying which) can derive a field from WHICH
	// wire name actually matched, without Go ever learning either name. The
	// transport that resolves the wire method (apidriver.translateLoop) is
	// the only place that knows it; it merges ByWire[matchedWire] into the
	// delivered payload as ordinary top-level fields, so map:'s existing
	// path grammar reads them exactly like any other payload field, no new
	// operator required.
	ByWire map[string]map[string]string `yaml:"by_wire"`
}

// HasChannelBlocks reports whether this event uses the channel-split form
// (api:/hooks: sub-blocks) rather than one flat in:/when:/map:.
func (e EventSpec) HasChannelBlocks() bool {
	return e.API != nil || e.Hooks != nil
}

// blockFor returns this event's own block for channel, or nil — either for a
// legacy event (no channel blocks at all) or for a channel-scoped event that
// simply declares nothing on THIS channel.
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

// WireEventFor returns the wire ref (and its direction) this event answers to
// on channel: the channel's own block if it declares one, else the event's
// legacy flat In/Out/Ask — a legacy event has not split by channel, so every
// channel reads the identical declaration. A channel-scoped event with no
// block for THIS channel reports Empty: the whole point of the split is that
// an undeclared channel is ABSENT, never silently satisfied by the other
// channel's shape.
//
// A block names at most one of In/Ask (checked at load), so trying both in
// turn is unambiguous — an ask-direction event's block has no In to prefer it
// over.
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

// WhenFor is WireEventFor's counterpart for the sum-type discriminator: the
// channel's own when: if it declares a block, else the event's legacy flat
// When (and nil for a channel-scoped event with no block on this channel).
func (e EventSpec) WhenFor(channel Channel) WhenMap {
	if block := e.blockFor(channel); block != nil {
		return block.When
	}
	if e.HasChannelBlocks() {
		return nil
	}
	return e.When
}

// AnyWireEvent returns the first wire ref this event declares in ANY form —
// legacy flat, or either channel block (in: or ask:) — for validation that
// only needs to know the event names something at all, not which channel.
func (e EventSpec) AnyWireEvent() (ref WireRef, direction string) {
	if wire, dir := e.WireEvent(); !wire.Empty() {
		return wire, dir
	}
	if wire, dir := e.WireEventFor(ChannelAPI); !wire.Empty() {
		return wire, dir
	}
	return e.WireEventFor(ChannelHooks)
}
