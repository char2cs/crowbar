use std::future::Future;
use std::pin::Pin;

use crate::error::Result;
use crate::stream::connection::Connection;

pub type Opening<'a> = Pin<Box<dyn Future<Output = Result<Connection>> + Send + 'a>>;

pub trait Dial: Send + Sync {
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
