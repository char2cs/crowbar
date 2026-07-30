#![forbid(unsafe_code)]

//! `crowbar-proto` — serde DTOs for the Go daemon's HTTP and WebSocket surface.
//!
//! Scaffold only (item 0.1). The contents of this crate are **generated** by
//! `native/tools/protogen` (item 0.5, spec §9.2) from the Go handlers; do not
//! hand-write types here once the generator lands.
//!
//! Dependency contract (§4.2): `serde` and nothing else.
