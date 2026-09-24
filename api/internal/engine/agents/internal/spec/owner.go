package spec

// Owner names which CHANNEL is authoritative for a dual-channel event —
// design spec P6b tag 1 (docs/plans/2026-09-22-descriptor-channel-split.md).
// Absent (EventSpec.Owner == "") means Either: no declared owner, so a
// delivery is never dropped on channel liveness alone. See
// Descriptor.EventOwner and turn/ingest.go's ownerDropsThisDelivery, which
// replaced apiOwnsThisEvent's static TransportFor(canonical) == "api" guess
// — the chat-theft root cause (design spec 1.1: transport is declared
// statically per event, but shape arrives per message).
const (
	OwnerAPI    = "api"
	OwnerHooks  = "hooks"
	OwnerEither = "either"
)
