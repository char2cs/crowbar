#![forbid(unsafe_code)]

//! `oracle` — the parity differ (§8.1).
//!
//! Scaffold only (item 0.1). Takes two snapshots of anchored geometry, one
//! from the React reference and one from the native app, and emits ranked
//! deltas.
//!
//! Dependency contract (§4.2): **none.** The oracle must not be able to import
//! the crates it judges.
//!
//! The logic lives here rather than in `main.rs` because §12 holds this crate
//! to ≥98% line coverage, which needs a library target to measure.
//!
//! `corpus/` and `blocked/` sit alongside this directory and are **not** part
//! of the crate. `corpus/` is append-only and ratcheted (§8.4); worker agents
//! may not write to either.
