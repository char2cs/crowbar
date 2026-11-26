//! Attachment to the daemon's browser peer socket: the connection loop, its frames in and out.

mod agent_request;
mod connection_event;
mod ending;
mod line;
mod outbound;
mod peer_connector;
mod peer_frame;
mod peer_session;
mod peer_stream;

pub(crate) use ending::Ending;
pub(crate) use line::serve;
pub use peer_connector::PeerConnector;
pub use peer_session::PeerSession;
pub use peer_stream::PeerStream;
