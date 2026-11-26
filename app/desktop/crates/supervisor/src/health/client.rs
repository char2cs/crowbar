//! HTTP over the daemon socket: the dial port, its socket adapter with fetch, and get.

pub(crate) mod dial;
mod get;
mod socket_dial;

pub(crate) use dial::Dial;
pub(crate) use get::get;
pub(crate) use socket_dial::SocketDial;
pub use socket_dial::fetch;
