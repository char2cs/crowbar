use std::sync::Arc;

use protocol::{DaemonRequest, DaemonResponse, DaemonTransport, RelayEndpoint, RelayScope, Responder};
use tokio::runtime::Handle;

use crate::daemon_wire::{from_http, refusal, to_http};
use crate::error::Error;
use crate::host_target::HostTarget;
use crate::stream_access::StreamAccess;
use crate::transport::Transport;

/// The [`DaemonTransport`] the shell holds: HTTP through [`Transport`], WebSocket access through
/// a [`StreamAccess`] (the loopback relay today).
pub struct ShellTransport {
    runtime: Handle,
    transport: Arc<Transport>,
    access: Arc<dyn StreamAccess>,
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
        }
    }
}

fn into_protocol(error: Error) -> protocol::Error {
    match error {
        Error::UnknownHost(id) => protocol::Error::UnknownHost(id),
        Error::BadGrant(why) => protocol::Error::Denied(why.to_owned()),
        other => protocol::Error::HostUnavailable(other.to_string()),
    }
}

async fn answer(transport: &Transport, request: &DaemonRequest) -> DaemonResponse {
    match to_http(request) {
        Ok((scope, http_request)) => from_http(&transport.handle(&scope, http_request).await),
        Err(e) => refusal(&e),
    }
}

impl DaemonTransport for ShellTransport {
    fn request(&self, request: DaemonRequest, respond: Responder) {
        let transport = Arc::clone(&self.transport);
        self.runtime.spawn(async move {
            respond(Ok(answer(&transport, &request).await));
        });
    }

    fn relay_endpoint(&self, scope: &RelayScope) -> protocol::Result<RelayEndpoint> {
        if let HostTarget::Remote(id) = HostTarget::of(&scope.host)
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
