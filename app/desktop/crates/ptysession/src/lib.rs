//! Connects a native terminal to the daemon: creates the PTY session over the shell's transport
//! and pumps its websocket through the loopback relay. No GPUI.

mod connection;
mod ends;
mod error;
mod journal_sink;
mod readiness;
mod report;
mod rest;
mod stream;
mod terminal_sessions;
#[cfg(test)]
mod test_support;

pub use connection::Connection;
pub use ends::Ends;
pub use error::{Error, Result};
pub use readiness::{Readiness, Wait};
pub use report::Report;
pub use rest::{Binding, SessionPaths};
pub use terminal_sessions::TerminalSessions;
