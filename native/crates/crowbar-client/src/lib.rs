#![forbid(unsafe_code)]

//! `crowbar-client` — the only thing in the tree that talks to the daemon.
//!
//! Scaffold only (item 0.1). Owns the unix-socket HTTP client, the WebSocket
//! connection, reconnect and backoff (spec §9.1). No domain logic lives here;
//! it belongs in `crowbar-core`.
//!
//! Dependency contract (§4.2): `crowbar-proto`.
