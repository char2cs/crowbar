#![forbid(unsafe_code)]

//! `crowbar-state` — `Entity<T>` stores, the event graph and subscription
//! wiring.
//!
//! Scaffold only (item 0.1). Awaiting the vendored `gpui` from item 0.2.
//!
//! Dependency contract (§4.2): `crowbar-core`, `crowbar-client`, plus `gpui`
//! when it lands.
//!
//! §7.1: domain state lives in `crowbar-core` as plain Rust. This crate holds
//! the `Entity<T>` wrapper and the subscription graph, nothing more. If a
//! store's logic can be tested without `gpui`, it belongs in `core` (D2).
