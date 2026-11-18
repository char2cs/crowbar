//! The channel pair between a view and its daemon session: the view end, the host end and frame batches.

mod frame_batch;
mod terminal_link;
mod terminal_peer;

pub(crate) use frame_batch::take_batch;
pub use terminal_link::TerminalLink;
pub use terminal_peer::TerminalPeer;
