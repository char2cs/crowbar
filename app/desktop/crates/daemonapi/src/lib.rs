//! The one typed client of a Crowbar daemon's API, over `protocol::DaemonTransport`.
//!
//! [`Client`] sends a [`Call`] to one host and reads the `{success, data, error}` envelope
//! every route answers with; the route builders name the daemon's paths. [`Dial`] opens the
//! daemon's `/v0/stream` ([`RelayDial`] through the shell's loopback relay) as a [`Connection`]
//! of [`Frame`]s.

mod client;
mod entry_kind;
mod error;
pub mod new_id;
mod problem;
pub mod routes;
mod stream;
mod switch;
#[cfg(feature = "fake")]
mod test_support;

pub use self::client::decode;
pub use self::client::envelope;
pub use self::client::multipart;
pub use self::client::read;
pub use self::stream::frame;
pub use client::{Call, Client};
pub use entry_kind::EntryKind;
pub use error::{Error, Result};
pub use problem::Problem;
pub use stream::{CORE_ORIGIN, Connection, Dial, Frame, Opening, RelayDial};
pub use switch::Switch;
#[cfg(feature = "fake")]
pub use test_support::{FakeDaemon, Seen};
