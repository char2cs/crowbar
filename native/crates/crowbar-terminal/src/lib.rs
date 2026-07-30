#![forbid(unsafe_code)]

//! `crowbar-terminal` — GPU text-grid renderer over the daemon's VT screen
//! model.
//!
//! Scaffold only (item 0.1).
//!
//! Dependency contract (§4.2): `crowbar-ui`, `crowbar-state`.
//!
//! The daemon owns the VT model (§10.2 rejects `alacritty_terminal`): this
//! crate renders rows it is given and injects input. It does not parse escape
//! sequences.
