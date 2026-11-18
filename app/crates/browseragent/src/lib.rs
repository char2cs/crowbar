//! The desktop side of the daemon's browser tools.
//!
//! Agents call `browser_open`, `browser_click` and the rest on the daemon, which hands each call
//! to the one window attached to its browser peer socket. This crate is that window's end:
//! [`run_peer`] keeps the attachment, reports the window's tabs ([`TabInfo`]) and turns every
//! request into an [`AgentCommand`] for the shell, which owns the tabs and answers through
//! [`Replies`]. It knows no window system: the connection ([`PeerConnector`]) and the shell
//! ([`TabCommands`]) are ports.

mod agent_command;
mod agent_op;
mod agent_request;
mod arming;
mod browseragent;
mod connection;
mod connection_event;
mod delivery;
mod dom_op;
mod ending;
mod error;
mod expectation;
mod line;
mod outbound;
mod peer_connector;
mod peer_frame;
mod peer_session;
mod peer_stream;
mod pending_forget;
mod pending_reply;
mod replies;
mod reply_scope;
mod tab_commands;
mod tab_info;
#[cfg(test)]
mod test_support;

pub use agent_command::AgentCommand;
pub use agent_op::AgentOp;
pub use arming::Arming;
pub use browseragent::run_peer;
pub use delivery::Delivery;
pub use dom_op::DomOp;
pub use error::Error;
pub use expectation::Expectation;
pub use peer_connector::PeerConnector;
pub use peer_session::PeerSession;
pub use peer_stream::PeerStream;
pub use replies::Replies;
pub use tab_commands::TabCommands;
pub use tab_info::TabInfo;
