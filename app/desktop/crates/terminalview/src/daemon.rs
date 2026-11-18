//! The channel pair between a view and its daemon session: the view end, the host end and frame batches.

mod frame_batch;
mod link;
mod peer;

pub(crate) use frame_batch::take_batch;
pub use link::Link;
pub use peer::Peer;
