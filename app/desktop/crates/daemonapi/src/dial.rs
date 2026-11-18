//! How a stream is opened.

use std::future::Future;
use std::pin::Pin;

use crate::connection::Connection;
use crate::error::Result;

/// The pending open of a stream: it resolves to the [`Connection`] or why it failed.
pub type Opening<'a> = Pin<Box<dyn Future<Output = Result<Connection>> + Send + 'a>>;

/// A way to open the daemon's `/v0/stream` as a [`Connection`].
pub trait Dial: Send + Sync {
    /// Connects, or says why it could not.
    fn open(&self) -> Opening<'_>;
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::error::Error;

    struct Refusing;

    impl Dial for Refusing {
        fn open(&self) -> Opening<'_> {
            Box::pin(std::future::ready(Err(Error::Stream("no route".into()))))
        }
    }

    #[tokio::test]
    async fn a_dial_that_cannot_connect_says_why() {
        let dial: &dyn Dial = &Refusing;
        let failed = dial.open().await.unwrap_err();
        assert_eq!(failed.to_string(), "daemon api: stream: no route");
    }
}
