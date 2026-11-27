use std::future::Future;

use crate::error::Error;

pub trait PeerStream: Send + 'static {
    fn recv(&mut self) -> impl Future<Output = Option<String>> + Send;

    fn send(
        &mut self,
        text: String,
    ) -> impl Future<Output = Result<(), Error>> + Send;
}
