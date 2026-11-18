//! The one typed client of a Crowbar daemon's API, over `protocol::DaemonTransport`.
//!
//! [`DaemonClient`] sends a [`Call`] to one host and reads the `{success, data, error}` envelope
//! every route answers with; the route builders name the daemon's paths. [`Dial`] opens the
//! daemon's `/v0/stream` ([`RelayDial`] through the shell's loopback relay) as a [`Connection`]
//! of [`Frame`]s.

mod call;
mod connection;
mod core_origin;
mod daemon_client;
pub mod daemon_read;
mod dial;
pub mod document_route;
mod entry_kind;
mod envelope;
mod error;
pub mod frame;
pub mod multipart;
pub mod new_id;
mod problem;
pub mod query_encode;
mod relay_dial;
pub mod routes;
mod switch;
#[cfg(feature = "fake")]
mod test_support;
pub mod typed;

pub use call::Call;
pub use connection::Connection;
pub use core_origin::CORE_ORIGIN;
pub use daemon_client::DaemonClient;
pub use dial::Dial;
pub use dial::Opening;
pub use entry_kind::EntryKind;
pub use error::{Error, Result};
pub use frame::Frame;
pub use problem::Problem;
pub use relay_dial::RelayDial;
pub use switch::Switch;
#[cfg(feature = "fake")]
pub use test_support::{FakeDaemon, Seen};
