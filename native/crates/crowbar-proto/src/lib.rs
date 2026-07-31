#![forbid(unsafe_code)]

//! `crowbar-proto` — serde DTOs for the Go daemon's HTTP and WebSocket surface.
//!
//! Everything under [`generated`] is **generated** by `native/tools/protogen`
//! (spec §9.2) from the Go handlers; do not hand-write or hand-edit types
//! there. Regenerate with `native/scripts/regen-proto.sh`, which is the single
//! command that reproduces this directory — flags, contract enforcement,
//! formatting and round-trip tests included.
//!
//! Dependency contract (§4.2): `serde` and nothing else.
//!
//! # What the wire shapes assume
//!
//! Every note below mirrors what `encoding/json` on the Go side actually does,
//! and each one is a decision the generator made rather than a default:
//!
//! * `time.Time` is a [`String`] in RFC 3339, as `time.Time`'s own
//!   `MarshalJSON` writes it. Keeping it a string is what lets this crate stay
//!   dependency-free and lossless — the daemon's exact rendering, offset and
//!   nanosecond precision included, round-trips byte for byte.
//! * `int64`/`uint64` are `i64`/`u64` *numbers*. The daemon uses no `,string`
//!   struct tags; this was checked rather than assumed.
//! * `[]byte` is a [`String`], because `encoding/json` base64-encodes it.
//! * A nil Go slice or map marshals as `null` rather than being omitted, and
//!   serde refuses `null` for `Vec`/`HashMap`. Non-optional container fields
//!   therefore deserialise through [`generated::null_default::null_to_default`]
//!   so a nil-on-the-wire collection arrives empty instead of failing the whole
//!   response.
//! * A named Go string type is an **open** set: its zero value `""` is legal
//!   and no constant declares it. Every generated enum therefore carries an
//!   untagged `Other(String)` variant. A closed enum would reject valid daemon
//!   output at runtime, which is the failure §9.2 exists to prevent.
//!
//! # Do not read this crate's coverage percentage as a measure of its tests
//!
//! `cargo llvm-cov -p crowbar-proto` reports on **six lines**: the body of
//! [`generated::null_default::null_to_default`]. Nothing else in the crate is
//! instrumented, because `rustc` excludes `#[automatically_derived]` items from
//! coverage, and every other line here is a struct field or an enum variant
//! whose only code is a derive. Measured, not assumed: a crate with one derived
//! and one hand-written `impl Clone`, with both exercised, reports the
//! hand-written one and not the derived one.
//!
//! So the number is 100% and it means almost nothing on its own. The tests that
//! do mean something are in `tests/`: `generated_roundtrip.rs` is emitted from
//! the same IR as the DTOs and drives every carryable declaration through its
//! wire fixture, both directions, plus its zero-value shape; `null_default.rs`
//! covers the one function that is code. If a future change makes the
//! percentage the gate rather than the suite, the gate can be satisfied by
//! deleting every DTO test and keeping three.
//!
//! # The one module the generator emitted that this crate cannot carry
//!
//! `gin.H` — gin's own `map[string]any` helper — lowers to
//! `HashMap<String, serde_json::Value>`, and `serde_json` is not on this
//! crate's dependency list and is not going on it. No Crowbar DTO references
//! it: it enters the closure only because five handlers still answer with an
//! untyped `gin.H` instead of a named DTO (see `native/protogen.manifest.json`,
//! category `untyped-payload`). `regen-proto.sh` drops that module and says so
//! on stderr; the fix is on the Go side, not here.

/// The generated wire DTOs, one module per Go package.
///
/// The `allow`s live on this declaration rather than inside the generated
/// files so that regenerating cannot silently drop them: generated code is not
/// hand-tuned to satisfy a style lint, and `clippy::pedantic` is denied
/// workspace-wide (§4.3 rule 4).
#[allow(non_snake_case)]
#[allow(clippy::pedantic, clippy::all)]
pub mod generated;

/// The generated modules are re-exported at the crate root, so a consumer
/// writes `crowbar_proto::domain_git::FileDiff` rather than threading
/// `generated::` through every path. The generated round-trip suite addresses
/// its types this way too, so removing this line breaks it.
pub use generated::*;
