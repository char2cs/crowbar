//! Connects a native terminal to the daemon: creates the PTY session over the shell's transport
//! and pumps its websocket through the loopback relay. No GPUI.
#![allow(
    clippy::match_bool,
    reason = "the manifest forbids else, so two-way branches use match"
)]
#![allow(
    clippy::too_many_arguments,
    reason = "the manifest limit does not count self; clippy's does"
)]

mod binding;
mod ends;
mod error;
mod reconnect;
mod rest;
mod session_base;
mod stream;
mod stream_end;
mod terminal_sessions;
mod wire;

pub use ends::Ends;
pub use error::{Error, Result};
pub use session_base::SessionBase;
pub use terminal_sessions::TerminalSessions;
