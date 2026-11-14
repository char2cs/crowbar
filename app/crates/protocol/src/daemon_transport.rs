use crate::daemon_request::DaemonRequest;
use crate::daemon_response::DaemonResponse;
use crate::error::Result;
use crate::relay_endpoint::RelayEndpoint;
use crate::relay_scope::RelayScope;

/// Completion callback of [`DaemonTransport::request`]; called exactly once.
pub type Responder = Box<dyn FnOnce(Result<DaemonResponse>) + Send + 'static>;

/// How the shell reaches daemon hosts.
///
/// The shell serves `crowbar://<host>/...` requests and hands out WebSocket relay endpoints
/// through this trait. The implementation (unix socket, paired remote host, token and TLS pin
/// handling) lives in the `transport` crate. The trait is callback based so it needs no async
/// runtime: an implementation may answer from any thread.
pub trait DaemonTransport: Send + Sync + 'static {
    /// Forwards `request` and calls `respond` exactly once, from any thread. Must not block
    /// the caller.
    fn request(&self, request: DaemonRequest, respond: Responder);

    /// Issues a relay endpoint and token for a pane's WebSocket streams.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the host is unknown or unreachable.
    fn relay_endpoint(&self, scope: &RelayScope) -> Result<RelayEndpoint>;
}

#[cfg(test)]
#[path = "daemon_transport_tests.rs"]
mod tests;
