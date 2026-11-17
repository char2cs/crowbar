//! The port that opens the byte stream a relay websocket runs over.

use std::future::Future;
use std::io;
use std::pin::Pin;
use std::sync::Arc;

use tokio::net::TcpStream;

use crate::duplex::Duplex;

pub(crate) type Link = Box<dyn Duplex>;

pub(crate) type Dialing = Pin<Box<dyn Future<Output = io::Result<Link>> + Send>>;

pub(crate) type Dial = Arc<dyn Fn(String) -> Dialing + Send + Sync>;

pub(crate) fn tcp_dial(address: String) -> Dialing {
    Box::pin(async move {
        let tcp = TcpStream::connect(address).await?;
        Ok(Box::new(tcp) as Link)
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn dialing_an_address_nothing_listens_on_fails() {
        assert!(tcp_dial("127.0.0.1:1".to_owned()).await.is_err());
    }
}
