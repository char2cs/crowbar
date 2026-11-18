//! What pairing needs of the transport: probing a host, pairing with it and forgetting it.

use std::future::Future;
use std::pin::Pin;

use crate::book_error::BookError;
use crate::pairing_request::PairingRequest;

/// A transport call that ends with a value or the transport's own error.
pub type BookFuture<T> = Pin<Box<dyn Future<Output = Result<T, BookError>> + Send>>;

/// The paired hosts as the transport keeps them (their addresses and keys).
pub trait HostBook: Send + Sync {
    /// The fingerprint of the certificate `endpoint` presents.
    fn probe(
        &self,
        endpoint: &str,
    ) -> BookFuture<String>;

    /// Redeems the code and registers the host; resolves to the id this pairing has on the host.
    fn pair(
        &self,
        request: PairingRequest,
    ) -> BookFuture<String>;

    /// Forgets the host's address and key.
    fn unpair(
        &self,
        host: &str,
    ) -> BookFuture<()>;
}
