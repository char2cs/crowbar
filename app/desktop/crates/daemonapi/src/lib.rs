//! The one typed client of a Crowbar daemon's API, over `protocol::DaemonTransport`.
//!
//! [`Client`] sends a [`Call`] to one host and reads the `{success, data, error}` envelope
//! every route answers with; a [`Route`] names a daemon path. [`Dial`] opens the
//! daemon's `/v0/stream` ([`RelayDial`] through the shell's loopback relay) as a [`Connection`]
//! of [`Frame`]s.

mod client;
mod entry_kind;
mod error;
mod problem;
mod route;
mod stream;
mod switch;
#[cfg(feature = "fake")]
mod test_support;
mod uuid;

pub use client::{Call, Client, Envelope};
pub use entry_kind::EntryKind;
pub use error::{Error, Result};
pub use problem::Problem;
pub use route::Route;
pub use stream::{CORE_ORIGIN, Connection, Dial, Frame, Opening, RelayDial};
pub use switch::Switch;
#[cfg(feature = "fake")]
pub use test_support::{FakeDaemon, Seen};
pub use uuid::Uuid;
