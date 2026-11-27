use std::future::Future;
use std::pin::Pin;

use crate::live::fleet::host_desk::book_error::BookError;
use crate::live::fleet::host_desk::pairing_request::PairingRequest;

pub type BookFuture<T> = Pin<Box<dyn Future<Output = Result<T, BookError>> + Send>>;

pub trait HostBook: Send + Sync {
    fn probe(
        &self,
        endpoint: &str,
    ) -> BookFuture<String>;

    fn pair(
        &self,
        request: PairingRequest,
    ) -> BookFuture<String>;

    fn unpair(
        &self,
        host: &str,
    ) -> BookFuture<()>;
}
