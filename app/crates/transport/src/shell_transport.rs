use std::sync::Arc;

use protocol::{
    DaemonRequest, DaemonResponse, DaemonTransport, HostId, RelayEndpoint, RelayScope, Responder,
};
use tokio::runtime::Handle;

use crate::daemon_wire::{from_http, refusal, to_http};
use crate::error::Error;
use crate::host_limits::HostLimits;
use crate::host_target::HostTarget;
use crate::reply::Reply;
use crate::stream_access::StreamAccess;
use crate::transport::{PROXY_TIMEOUT, Transport};

/// The [`DaemonTransport`] the shell holds: HTTP through [`Transport`], WebSocket access through
/// a [`StreamAccess`] (the loopback relay today).
///
/// A request is answered once, from the runtime given to [`ShellTransport::new`]; if that
/// runtime stops first the pane gets `Interrupted`. At most 32 requests per host are in flight,
/// and the rest wait their turn within the proxy timeout.
pub struct ShellTransport {
    runtime: Handle,
    transport: Arc<Transport>,
    access: Arc<dyn StreamAccess>,
    limits: Arc<HostLimits>,
}

impl std::fmt::Debug for ShellTransport {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("ShellTransport").finish_non_exhaustive()
    }
}

impl ShellTransport {
    /// Serves requests on `runtime`, which the shell keeps alive for the app's life.
    #[must_use]
    pub fn new(runtime: Handle, transport: Arc<Transport>, access: Arc<dyn StreamAccess>) -> Self {
        Self {
            runtime,
            transport,
            access,
            limits: Arc::new(HostLimits::default()),
        }
    }
}

pub(crate) fn into_protocol(error: Error) -> protocol::Error {
    match error {
        Error::UnknownHost(id) => protocol::Error::UnknownHost(id),
        other => protocol::Error::HostUnavailable(Box::new(other)),
    }
}

async fn answer(
    transport: &Transport,
    limits: &HostLimits,
    request: DaemonRequest,
) -> DaemonResponse {
    let host: HostId = request.host().clone();
    let queued = tokio::time::timeout(PROXY_TIMEOUT, limits.acquire(&host)).await;
    let Ok(Some(_slot)) = queued else {
        return from_http(crate::response::error_response(
            503,
            "too many requests are waiting for this host",
        ));
    };
    match to_http(request) {
        Ok((origin, http_request)) => from_http(transport.handle(&origin, http_request).await),
        Err(e) => refusal(&e),
    }
}

impl DaemonTransport for ShellTransport {
    fn request(&self, request: DaemonRequest, respond: Responder) {
        let reply = Reply::new(respond);
        let transport = Arc::clone(&self.transport);
        let limits = Arc::clone(&self.limits);
        self.runtime.spawn(async move {
            reply.send(answer(&transport, &limits, request).await);
        });
    }

    fn relay_endpoint(&self, scope: &RelayScope) -> protocol::Result<RelayEndpoint> {
        if let HostTarget::Remote(id) = HostTarget::of(scope.host())
            && self.transport.registry().get(&id).is_none()
        {
            return Err(protocol::Error::UnknownHost(id));
        }
        self.access.issue(scope).map_err(into_protocol)
    }
}

#[cfg(test)]
#[path = "shell_transport_tests.rs"]
mod tests;
