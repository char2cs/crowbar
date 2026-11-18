//! Web sockets to a daemon: the upstream dial and text streams.

#[cfg(feature = "loopback")]
mod text_loopback;
mod text_stream;
mod upstream;
mod upstream_request;

pub use text_stream::TextStream;
pub(crate) use upstream::{UpstreamSocket, ws_config};
pub(crate) use upstream_request::UpstreamRequest;
