use std::future::Future;

use crate::error::Error;
use crate::link::peer_stream::PeerStream;

pub trait PeerConnector: Send + Sync + 'static {
    type Stream: PeerStream;

    fn connect(&self) -> impl Future<Output = Result<Self::Stream, Error>> + Send;
}
