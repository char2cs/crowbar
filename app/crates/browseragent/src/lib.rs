//! The desktop side of the daemon's browser tools.
//!
//! Agents call `browser_open`, `browser_click` and the rest on the daemon, which hands each call
//! to the one window attached to its browser peer socket. This crate is that window's end:
//! [`run_peer`] keeps the attachment, reports the window's tabs ([`TabInfo`]) and turns every
//! request into an [`AgentCommand`] for the shell, which owns the tabs and answers through
//! [`Replies`]. It knows no window system: the connection ([`PeerConnector`]) and the shell
//! ([`TabCommands`]) are ports.

mod browseragent;
mod error;
mod link;
mod shell;
#[cfg(test)]
mod test_support;
mod wait;

pub use browseragent::run_peer;
pub use error::Error;
pub use link::{PeerConnector, PeerSession, PeerStream};
pub use shell::{AgentCommand, AgentOp, DomOp, TabCommands, TabInfo};
pub use wait::{Arming, Delivery, Expectation, Replies};
