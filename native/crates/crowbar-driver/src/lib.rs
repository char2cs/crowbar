#![forbid(unsafe_code)]

//! `crowbar-driver` — the in-process test driver: element-tree extractor, input
//! injector and MCP server over stdio (§10.4, D7).
//!
//! Scaffold only (item 0.1).
//!
//! Dependency contract (§4.2): everything.
//!
//! **Feature-gated.** This crate is an *optional* dependency of `crowbar-app`
//! behind that binary's `driver` feature, so a default build links none of it.
//! Build the oracle against `--release --features driver` so the optimisation
//! level matches shipping.
//!
//! The driver adds a control channel; **it must not alter rendering.** If it
//! ever does, that is a bug of the highest severity — every oracle result taken
//! while it was true is void.
