//! Connects a native terminal to the daemon: creates the PTY session over the shell's transport
//! and pumps its websocket through the loopback relay. No GPUI.

mod ends;
mod error;
mod rest;
mod stream;
mod terminal_sessions;
#[cfg(test)]
mod test_support;

pub use ends::Ends;
pub use error::{Error, Result};
pub use rest::SessionBase;
pub use terminal_sessions::TerminalSessions;
