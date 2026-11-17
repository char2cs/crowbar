//! Connects a native terminal to the daemon: creates the PTY session over the shell's transport
//! and pumps its websocket through the loopback relay. No GPUI.

mod attachment;
mod binding;
mod dial;
mod duplex;
mod ends;
mod error;
mod handshake;
mod reconnect;
mod recovery;
mod rest;
mod session_base;
mod stream;
mod stream_end;
mod terminal_sessions;
#[cfg(test)]
mod test_support;
mod wire;

pub use ends::Ends;
pub use error::{Error, Result};
pub use session_base::SessionBase;
pub use terminal_sessions::TerminalSessions;
