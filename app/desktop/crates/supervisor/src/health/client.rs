//! HTTP over the daemon socket: the dial port, its socket adapter, get and fetch.

pub(crate) mod dial;
mod fetch;
mod get;
mod socket_dial;

pub(crate) use dial::Dial;
pub use fetch::fetch;
pub(crate) use get::get;
pub(crate) use socket_dial::SocketDial;
